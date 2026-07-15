package onvif

import (
	"bufio"
	"context"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	onvifc "github.com/cedricve/go-onvif"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// scanPort describes a TCP port we probe while scanning the local network,
// together with a human readable service name.
type scanPort struct {
	Port    int
	Service string
	// rtsp marks RTSP ports we can fingerprint via an OPTIONS request.
	rtsp bool
	// http marks HTTP ports we can fingerprint via a banner grab.
	http bool
	// camera marks ports that strongly hint the device is an IP camera or NVR
	// (RTSP, dedicated ONVIF ports and well-known DVR/NVR control ports).
	camera bool
}

// commonCameraPorts is the list of TCP ports we probe on every host. These are
// the ports most commonly exposed by IP cameras (RTSP, HTTP(S) and ONVIF).
var commonCameraPorts = []scanPort{
	{Port: 554, Service: "RTSP", rtsp: true, camera: true},
	{Port: 8554, Service: "RTSP (alt)", rtsp: true, camera: true},
	{Port: 80, Service: "HTTP", http: true},
	{Port: 8080, Service: "HTTP (alt)", http: true},
	{Port: 8000, Service: "ONVIF", http: true, camera: true},
	{Port: 8899, Service: "ONVIF (alt)", camera: true},
	{Port: 443, Service: "HTTPS"},
	{Port: 37777, Service: "Dahua", camera: true},
	{Port: 34567, Service: "XMeye/Sofia", camera: true},
}

// ouiVendors maps the first three octets (OUI) of a MAC address, upper-cased and
// without separators, to a known camera/NVR vendor. This lets us flag likely
// cameras the same way tools such as Fing or WiFiman do, even when a device does
// not answer to ONVIF WS-Discovery.
var ouiVendors = map[string]string{
	"BCAD01": "Hikvision", "C056E3": "Hikvision", "4CBD8F": "Hikvision",
	"44A642": "Hikvision", "E0509B": "Hikvision", "ACB927": "Hikvision",
	"18800C": "Hikvision", "C40BCB": "Hikvision",
	"3CEF8C": "Dahua", "90020A": "Dahua", "E0509B00": "Dahua",
	"08ED02": "Dahua", "3CE376": "Dahua", "38AF29": "Dahua", "E45D51": "Dahua",
	"00408C": "Axis", "AABBCC": "Axis", "B8A44F": "Axis", "ACCC8E": "Axis",
	"E82725": "Bosch", "000CAB": "Bosch",
	"001B9E": "Hanwha", "0009D2": "Hanwha", "E44CC7": "Hanwha",
	"EC7196": "Reolink", "9CA3BA": "Reolink",
	"3C33F1": "Amcrest", "9C8ECD": "Amcrest",
	"000FFC": "Vivotek", "0002D1": "Vivotek",
	"001C27": "Mobotix", "0003C5": "Mobotix",
	"00126A": "Ubiquiti", "FCECDA": "Ubiquiti", "744401": "Ubiquiti",
	"F0234B": "Foscam", "00626E": "Foscam",
	"C09424": "TP-Link", "50C7BF": "TP-Link",
}

// DiscoverDevices performs an advanced, Fing/WiFiman-style scan of the local
// network. It combines:
//
//  1. ONVIF WS-Discovery (multicast probe), and
//  2. an active TCP port scan of every host on the local IPv4 subnets for the
//     ports typically exposed by IP cameras, and
//  3. MAC address + vendor (OUI) resolution from the local ARP table, and
//  4. best-effort reverse-DNS hostname lookup.
//
// The results are merged per IP address so a single device is reported once
// with all the information we could gather. Devices are flagged as cameras when
// they answer to ONVIF, expose an RTSP port, or have a MAC that belongs to a
// known camera vendor.
//
// Optional subnets (CIDR notation, e.g. "192.168.1.0/24") override the
// automatically detected local subnets. This is useful when the agent runs in a
// container/devcontainer whose interfaces are not on the same range as the
// cameras, but the target range is still routable from the host network.
func DiscoverDevices(timeout time.Duration, subnets ...string) []models.DiscoveredDevice {
	devicesByIP := make(map[string]*models.DiscoveredDevice)
	var mutex sync.Mutex

	// upsert returns the (possibly newly created) device entry for an IP in a
	// concurrency-safe way.
	upsert := func(ip string) *models.DiscoveredDevice {
		mutex.Lock()
		defer mutex.Unlock()
		device, ok := devicesByIP[ip]
		if !ok {
			device = &models.DiscoveredDevice{IP: ip}
			devicesByIP[ip] = device
		}
		return device
	}

	// 1) ONVIF WS-Discovery. This is quick and reliable for ONVIF cameras.
	onvifDevices, err := onvifc.StartDiscovery(timeout)
	if err != nil {
		log.Log.Error("onvif.DiscoverDevices(): WS-Discovery failed: " + err.Error())
	} else {
		for _, onvifDevice := range onvifDevices {
			ip := hostFromXAddr(onvifDevice.XAddr)
			if ip == "" {
				continue
			}
			device := upsert(ip)
			device.ONVIF = true
			device.ONVIFXAddr = onvifDevice.XAddr
			device.IsCamera = true
			if hostname, hostErr := onvifDevice.GetHostname(); hostErr == nil && hostname.Name != "" {
				device.Hostname = hostname.Name
			}
		}
	}

	// 2) Active port scan across the requested (or auto-detected) IPv4 subnets.
	var targets []string
	if len(subnets) > 0 {
		targets = targetsFromSubnets(subnets)
	} else {
		targets = localScanTargets()
	}
	log.Log.Info("onvif.DiscoverDevices(): scanning " + strconv.Itoa(len(targets)) + " hosts on the local network(s)")

	// Bound the amount of concurrent dials so we do not exhaust file
	// descriptors on constrained devices (e.g. Raspberry Pi).
	semaphore := make(chan struct{}, 128)
	dialTimeout := perHostTimeout(timeout)
	var waitGroup sync.WaitGroup

	for _, ip := range targets {
		waitGroup.Add(1)
		semaphore <- struct{}{}
		go func(ip string) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()

			openPorts, services, isCamera := scanHost(ip, dialTimeout)
			if len(openPorts) == 0 {
				return
			}

			// Fingerprint the host (RTSP/HTTP banner grab) to determine its
			// manufacturer, model and type without any credentials.
			fingerprint := fingerprintHost(ip, openPorts, dialTimeout)

			device := upsert(ip)
			mutex.Lock()
			device.OpenPorts = mergeSortedInts(device.OpenPorts, openPorts)
			device.Services = mergeUniqueStrings(device.Services, services)
			if isCamera || fingerprint.IsCamera {
				device.IsCamera = true
			}
			if fingerprint.Manufacturer != "" {
				device.Manufacturer = fingerprint.Manufacturer
			}
			if fingerprint.Model != "" {
				device.Model = fingerprint.Model
			}
			if fingerprint.Type != "" {
				device.Type = fingerprint.Type
			}
			if fingerprint.Server != "" {
				device.Server = fingerprint.Server
			}
			for _, port := range openPorts {
				if port == 554 || port == 8554 {
					device.RTSPURL = "rtsp://" + ip + ":" + strconv.Itoa(port) + "/"
					break
				}
			}
			mutex.Unlock()
		}(ip)
	}
	waitGroup.Wait()

	// 3) Enrich with MAC address / vendor from the ARP table and hostnames.
	arpTable := readARPTable()
	results := make([]models.DiscoveredDevice, 0, len(devicesByIP))
	for ip, device := range devicesByIP {
		if mac, ok := arpTable[ip]; ok {
			device.MAC = mac
			if vendor := vendorFromMAC(mac); vendor != "" {
				device.Vendor = vendor
				device.IsCamera = true
			}
		}
		// Fall back to the MAC vendor for the manufacturer, and make sure a
		// camera always carries a device type.
		if device.Manufacturer == "" && device.Vendor != "" {
			device.Manufacturer = device.Vendor
		}
		if device.IsCamera && device.Type == "" {
			device.Type = "IP Camera"
		}
		if device.Hostname == "" {
			device.Hostname = reverseDNS(ip, dialTimeout)
		}
		results = append(results, *device)
	}

	// Cameras first, then by IP, for a stable and useful ordering.
	sort.Slice(results, func(i, j int) bool {
		if results[i].IsCamera != results[j].IsCamera {
			return results[i].IsCamera
		}
		return ipLess(results[i].IP, results[j].IP)
	})

	return results
}

// scanHost probes the common camera ports on a single host and reports the open
// ports, their service names, and whether the host looks like a camera.
func scanHost(ip string, dialTimeout time.Duration) (openPorts []int, services []string, isCamera bool) {
	for _, candidate := range commonCameraPorts {
		address := net.JoinHostPort(ip, strconv.Itoa(candidate.Port))
		conn, err := net.DialTimeout("tcp", address, dialTimeout)
		if err != nil {
			continue
		}
		conn.Close()
		openPorts = append(openPorts, candidate.Port)
		services = append(services, candidate.Service)
		if candidate.camera {
			isCamera = true
		}
	}
	return openPorts, services, isCamera
}

// targetsFromSubnets expands one or more explicit CIDR ranges (e.g.
// "192.168.1.0/24") into a de-duplicated list of host addresses. Invalid or
// oversized ranges (mask < /22) are skipped so scans stay bounded.
func targetsFromSubnets(subnets []string) []string {
	seen := make(map[string]struct{})
	var targets []string

	for _, subnet := range subnets {
		subnet = strings.TrimSpace(subnet)
		if subnet == "" {
			continue
		}
		// Allow passing a bare host address (e.g. "192.168.1.50") too.
		if !strings.Contains(subnet, "/") {
			if net.ParseIP(subnet).To4() != nil {
				if _, exists := seen[subnet]; !exists {
					seen[subnet] = struct{}{}
					targets = append(targets, subnet)
				}
			} else {
				log.Log.Error("onvif.targetsFromSubnets(): invalid address '" + subnet + "'")
			}
			continue
		}
		_, ipNet, err := net.ParseCIDR(subnet)
		if err != nil || ipNet.IP.To4() == nil {
			log.Log.Error("onvif.targetsFromSubnets(): invalid CIDR '" + subnet + "'")
			continue
		}
		if ones, bits := ipNet.Mask.Size(); bits != 32 || ones < 22 {
			log.Log.Error("onvif.targetsFromSubnets(): range '" + subnet + "' is too large to scan (use /22 or smaller)")
			continue
		}
		for _, host := range hostsInNetwork(ipNet) {
			if _, exists := seen[host]; exists {
				continue
			}
			seen[host] = struct{}{}
			targets = append(targets, host)
		}
	}
	return targets
}

// localScanTargets enumerates every usable IPv4 host address on the local
// network interfaces. To keep scans bounded we only expand subnets with a mask
// of /22 or smaller (at most ~1022 hosts per interface).
func localScanTargets() []string {
	seen := make(map[string]struct{})
	var targets []string

	interfaces, err := net.Interfaces()
	if err != nil {
		log.Log.Error("onvif.localScanTargets(): " + err.Error())
		return targets
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			if bits != 32 || ones < 22 {
				// Skip huge or non-IPv4 ranges to avoid endless scans.
				continue
			}
			for _, host := range hostsInNetwork(ipNet) {
				if _, exists := seen[host]; exists {
					continue
				}
				seen[host] = struct{}{}
				targets = append(targets, host)
			}
		}
	}
	return targets
}

// hostsInNetwork returns all assignable host addresses in the given network,
// excluding the network and broadcast addresses.
func hostsInNetwork(ipNet *net.IPNet) []string {
	var hosts []string
	network := ipNet.IP.Mask(ipNet.Mask).To4()
	if network == nil {
		return hosts
	}

	for ip := cloneIP(network); ipNet.Contains(ip); incrementIP(ip) {
		hosts = append(hosts, ip.String())
	}
	// Drop network + broadcast addresses when present.
	if len(hosts) > 2 {
		hosts = hosts[1 : len(hosts)-1]
	}
	return hosts
}

func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

// hostFromXAddr extracts the host (IP) part from an ONVIF XAddr URL such as
// "http://192.168.1.69:8000/onvif/device_service".
func hostFromXAddr(xaddr string) string {
	parsed, err := url.Parse(xaddr)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		// Fall back to a naive split for values without a scheme.
		host = strings.TrimPrefix(xaddr, "//")
		if idx := strings.IndexAny(host, ":/"); idx >= 0 {
			host = host[:idx]
		}
	}
	return host
}

// readARPTable parses /proc/net/arp (Linux) and returns a map of IP -> MAC. On
// non-Linux platforms or when the file is unavailable it returns an empty map.
func readARPTable() map[string]string {
	table := make(map[string]string)
	file, err := os.Open("/proc/net/arp")
	if err != nil {
		return table
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip the header line.
	if scanner.Scan() {
		_ = scanner.Text()
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		mac := fields[3]
		if mac == "00:00:00:00:00:00" || mac == "" {
			continue
		}
		table[ip] = strings.ToLower(mac)
	}
	return table
}

// vendorFromMAC resolves a MAC address to a known camera vendor using its OUI.
func vendorFromMAC(mac string) string {
	normalized := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "").Replace(mac))
	if len(normalized) < 6 {
		return ""
	}
	// Try a longer prefix first (some vendors share the first 3 octets).
	if len(normalized) >= 8 {
		if vendor, ok := ouiVendors[normalized[:8]]; ok {
			return vendor
		}
	}
	if vendor, ok := ouiVendors[normalized[:6]]; ok {
		return vendor
	}
	return ""
}

// reverseDNS performs a best-effort, time-bounded reverse DNS lookup.
func reverseDNS(ip string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var resolver net.Resolver
	names, err := resolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// perHostTimeout derives a short per-connection dial timeout from the overall
// discovery timeout, clamped to a sensible range.
func perHostTimeout(timeout time.Duration) time.Duration {
	dialTimeout := timeout / 4
	if dialTimeout < 300*time.Millisecond {
		dialTimeout = 300 * time.Millisecond
	}
	if dialTimeout > 1500*time.Millisecond {
		dialTimeout = 1500 * time.Millisecond
	}
	return dialTimeout
}

func mergeSortedInts(existing, added []int) []int {
	set := make(map[int]struct{}, len(existing)+len(added))
	for _, value := range existing {
		set[value] = struct{}{}
	}
	for _, value := range added {
		set[value] = struct{}{}
	}
	merged := make([]int, 0, len(set))
	for value := range set {
		merged = append(merged, value)
	}
	sort.Ints(merged)
	return merged
}

func mergeUniqueStrings(existing, added []string) []string {
	set := make(map[string]struct{}, len(existing)+len(added))
	merged := make([]string, 0, len(existing)+len(added))
	for _, value := range append(append([]string{}, existing...), added...) {
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}

// ipLess compares two IPv4 address strings numerically.
func ipLess(a, b string) bool {
	ipA := net.ParseIP(a).To4()
	ipB := net.ParseIP(b).To4()
	if ipA == nil || ipB == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if ipA[i] != ipB[i] {
			return ipA[i] < ipB[i]
		}
	}
	return false
}
