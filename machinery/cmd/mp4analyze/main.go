package main

import (
	"fmt"
	"os"

	"github.com/Eyevinn/mp4ff/avc"
	mp4ff "github.com/Eyevinn/mp4ff/mp4"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: mp4analyze <file.mp4>")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	parsed, err := mp4ff.DecodeFile(f)
	if err != nil {
		panic(err)
	}

	// Movie-level info
	if parsed.Init != nil && parsed.Init.Moov != nil {
		moov := parsed.Init.Moov
		fmt.Printf("ftyp/moov present. timescale(mvhd)=%d duration(mvhd)=%d\n",
			moov.Mvhd.Timescale, moov.Mvhd.Duration)
		for _, trak := range moov.Traks {
			ts := trak.Mdia.Mdhd.Timescale
			fmt.Printf("  trak id=%d handler=%s mdhd.timescale=%d mdhd.duration=%d\n",
				trak.Tkhd.TrackID, trak.Mdia.Hdlr.HandlerType, ts, trak.Mdia.Mdhd.Duration)
		}
	} else {
		fmt.Println("no Init/Moov (pure fragmented stream?)")
	}

	// sidx vs actual segment layout. MSE players use sidx to map presentation
	// time -> byte ranges; if sidx references disagree with the real segment
	// sizes/durations (e.g. after an early/short flush) the player fetches the
	// wrong bytes and fails to decode — a failure that "heals" on seek.
	fmt.Println("=== sidx references vs actual segments ===")
	var sidxRefs []mp4ff.SidxRef
	for _, c := range parsed.Children {
		if s, ok := c.(*mp4ff.SidxBox); ok {
			fmt.Printf("  sidx: timescale=%d earliestPresTime=%d firstOffset=%d refCount=%d anchor(after sidx)=%d\n",
				s.Timescale, s.EarliestPresentationTime, s.FirstOffset, len(s.SidxRefs), s.AnchorPoint)
			sidxRefs = s.SidxRefs
		}
	}
	// Actual segment sizes (styp+moof+mdat) and fragment durations.
	type segInfo struct {
		size uint64
		dur  uint64
	}
	var actual []segInfo
	for _, seg := range parsed.Segments {
		var sz uint64
		if seg.Styp != nil {
			sz += seg.Styp.Size()
		}
		if seg.Sidx != nil {
			sz += seg.Sidx.Size()
		}
		var dur uint64
		for _, fr := range seg.Fragments {
			sz += fr.Moof.Size()
			if fr.Mdat != nil {
				sz += fr.Mdat.Size()
			}
			for _, traf := range fr.Moof.Trafs {
				if traf.Tfhd.TrackID != 1 {
					continue
				}
				for _, trun := range traf.Truns {
					for _, s := range trun.Samples {
						dur += uint64(s.Dur)
					}
				}
			}
		}
		actual = append(actual, segInfo{size: sz, dur: dur})
	}
	for i := range actual {
		refStr := "(no sidx ref)"
		if i < len(sidxRefs) {
			r := sidxRefs[i]
			mark := ""
			if uint64(r.ReferencedSize) != actual[i].size {
				mark += fmt.Sprintf(" SIZE MISMATCH actual=%d", actual[i].size)
			}
			if uint64(r.SubSegmentDuration) != actual[i].dur {
				mark += fmt.Sprintf(" DUR MISMATCH actual=%d", actual[i].dur)
			}
			refStr = fmt.Sprintf("sidx.size=%d sidx.dur=%d type=%d sap=%d/%d%s",
				r.ReferencedSize, r.SubSegmentDuration, r.ReferenceType, r.StartsWithSAP, r.SAPType, mark)
		}
		fmt.Printf("  seg%02d actual.size=%d actual.dur=%d | %s\n", i, actual[i].size, actual[i].dur, refStr)
	}

	fmt.Println("=== fragments ===")
	fragIdx := 0
	var allKeyGlobal []uint64 // global keyframe decode times (track timescale units)
	var prevTfdtEnd = map[uint32]uint64{}
	for si, seg := range parsed.Segments {
		for _, fr := range seg.Fragments {
			for _, traf := range fr.Moof.Trafs {
				tid := traf.Tfhd.TrackID
				tfdt := traf.Tfdt.BaseMediaDecodeTime()
				offset := uint64(0)
				var keys []uint64    // keyframe offset-from-tfdt
				var durs []uint64
				zeroDur := 0
				nSamples := 0
				for _, trun := range traf.Truns {
					for _, s := range trun.Samples {
						nSamples++
						if (s.Flags>>24)&0x03 == 0x02 { // sample_depends_on==2 => IDR/sync
							keys = append(keys, offset)
							if tid == 1 {
								allKeyGlobal = append(allKeyGlobal, tfdt+offset)
							}
						}
						if s.Dur == 0 {
							zeroDur++
						}
						durs = append(durs, uint64(s.Dur))
						offset += uint64(s.Dur)
					}
				}
				cont := ""
				if pe, ok := prevTfdtEnd[tid]; ok {
					if tfdt != pe {
						cont = fmt.Sprintf(" <-- tfdt GAP/JUMP prev_end=%d delta=%d", pe, int64(tfdt)-int64(pe))
					}
				}
				prevTfdtEnd[tid] = tfdt + offset
				if tid == 1 {
					// in-fragment keyframe gaps
					var gaps []int64
					for i := 1; i < len(keys); i++ {
						gaps = append(gaps, int64(keys[i])-int64(keys[i-1]))
					}
					fmt.Printf("seg%d frag%d trk%d tfdt=%d dur=%d nSamp=%d zeroDur=%d keys=%v inFragKeyGaps=%v%s\n",
						si, fragIdx, tid, tfdt, offset, nSamples, zeroDur, keys, gaps, cont)
				}
			}
			fragIdx++
		}
	}

	fmt.Println("=== global video keyframe decode times & gaps ===")
	for i, k := range allKeyGlobal {
		gap := int64(0)
		if i > 0 {
			gap = int64(k) - int64(allKeyGlobal[i-1])
		}
		flag := ""
		if i > 1 {
			prevGap := int64(allKeyGlobal[i-1]) - int64(allKeyGlobal[i-2])
			if gap > 0 && prevGap > 0 && gap*2 < prevGap {
				flag = fmt.Sprintf("  <== SEAM? gap=%d < prevGap/2=%d", gap, prevGap/2)
			}
		}
		fmt.Printf("  kf#%02d dt=%d gap=%d%s\n", i, k, gap, flag)
	}

	// Full sample timeline: DTS, CTS (=DTS+cto), composition offset, NAL types,
	// to detect PTS non-monotonicity / gaps / param-set changes at the seam.
	fmt.Println("=== per-sample timeline (full) — checking PTS monotonicity & nal types ===")
	var trex *mp4ff.TrexBox
	if parsed.Init != nil && parsed.Init.Moov != nil && parsed.Init.Moov.Mvex != nil {
		for _, t := range parsed.Init.Moov.Mvex.Trexs {
			if t.TrackID == 1 {
				trex = t
			}
		}
	}
	var lastCTS int64 = -1
	var lastDTS int64 = -1
	sampIdx := 0
	fragIdx = 0
	for _, seg := range parsed.Segments {
		for _, fr := range seg.Fragments {
			fs, err := fr.GetFullSamples(trex)
			if err != nil {
				fmt.Printf("  frag%d GetFullSamples err: %v\n", fragIdx, err)
				fragIdx++
				continue
			}
			for _, s := range fs {
				dts := int64(s.DecodeTime)
				cts := dts + int64(s.CompositionTimeOffset)
				nals := nalTypes(s.Data)
				anomaly := ""
				if lastCTS >= 0 && cts < lastCTS {
					anomaly += fmt.Sprintf(" <== CTS BACKWARDS (prev=%d)", lastCTS)
				}
				if lastDTS >= 0 && dts < lastDTS {
					anomaly += fmt.Sprintf(" <== DTS BACKWARDS (prev=%d)", lastDTS)
				}
				isSync := s.Flags&0x02000000 == 0 && (s.Flags>>24)&0x03 == 0x02
				// Only print near the seam region and any anomalies, to keep output small.
				near := dts >= 7800 && dts <= 8700
				if near || anomaly != "" {
					fmt.Printf("  s%04d frag%d dts=%d cts=%d cto=%d dur=%d size=%d sync=%v nal=%v%s\n",
						sampIdx, fragIdx, dts, cts, s.CompositionTimeOffset, s.Dur, len(s.Data), isSync, nals, anomaly)
				}
				lastCTS = cts
				lastDTS = dts
				sampIdx++
			}
			fragIdx++
		}
	}

	// Compare parameter sets: avcC (in moov) vs inline SPS/PPS at every IDR.
	// A looping source that restarts may re-emit SPS/PPS that differ from the
	// ones the player configured its decoder with from avcC — a classic cause
	// of a freeze that "heals" when you seek past the seam.
	fmt.Println("=== parameter set comparison (avcC vs inline IDR) ===")
	var avccSPS, avccPPS [][]byte
	if parsed.Init != nil && parsed.Init.Moov != nil {
		for _, trak := range parsed.Init.Moov.Traks {
			if trak.Mdia == nil || trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil {
				continue
			}
			stsd := trak.Mdia.Minf.Stbl.Stsd
			if stsd == nil || stsd.AvcX == nil || stsd.AvcX.AvcC == nil {
				continue
			}
			avccSPS = stsd.AvcX.AvcC.SPSnalus
			avccPPS = stsd.AvcX.AvcC.PPSnalus
		}
	}
	for i, s := range avccSPS {
		fmt.Printf("  avcC SPS[%d] = %x\n", i, s)
	}
	for i, p := range avccPPS {
		fmt.Printf("  avcC PPS[%d] = %x\n", i, p)
	}
	fragIdx = 0
	sampIdx = 0
	var baseSPS, basePPS []byte
	if len(avccSPS) > 0 {
		baseSPS = avccSPS[0]
	}
	if len(avccPPS) > 0 {
		basePPS = avccPPS[0]
	}
	for _, seg := range parsed.Segments {
		for _, fr := range seg.Fragments {
			fs, err := fr.GetFullSamples(trex)
			if err != nil {
				fragIdx++
				continue
			}
			for _, s := range fs {
				spsList := nalsByType(s.Data, 7)
				ppsList := nalsByType(s.Data, 8)
				if len(spsList) > 0 || len(ppsList) > 0 {
					dts := int64(s.DecodeTime)
					note := ""
					if len(spsList) > 0 {
						if baseSPS == nil {
							baseSPS = spsList[0]
						} else if !bytesEqual(baseSPS, spsList[0]) {
							note += " <== SPS CHANGED vs base/avcC"
						}
					}
					if len(ppsList) > 0 {
						if basePPS == nil {
							basePPS = ppsList[0]
						} else if !bytesEqual(basePPS, ppsList[0]) {
							note += " <== PPS CHANGED vs base/avcC"
						}
					}
					var spsHex, ppsHex string
					if len(spsList) > 0 {
						spsHex = fmt.Sprintf("%x", spsList[0])
					}
					if len(ppsList) > 0 {
						ppsHex = fmt.Sprintf("%x", ppsList[0])
					}
					fmt.Printf("  IDR s%04d frag%d dts=%d SPS=%s PPS=%s%s\n",
						sampIdx, fragIdx, dts, spsHex, ppsHex, note)
				}
				sampIdx++
			}
			fragIdx++
		}
	}

	sliceHeaders(parsed, trex)
}

func sliceHeaders(parsed *mp4ff.File, trex *mp4ff.TrexBox) {
	// Build SPS/PPS maps from avcC.
	spsMap := map[uint32]*avc.SPS{}
	ppsMap := map[uint32]*avc.PPS{}
	if parsed.Init != nil && parsed.Init.Moov != nil {
		for _, trak := range parsed.Init.Moov.Traks {
			if trak.Mdia == nil || trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil {
				continue
			}
			stsd := trak.Mdia.Minf.Stbl.Stsd
			if stsd == nil || stsd.AvcX == nil || stsd.AvcX.AvcC == nil {
				continue
			}
			for _, s := range stsd.AvcX.AvcC.SPSnalus {
				if sps, err := avc.ParseSPSNALUnit(s, true); err == nil {
					spsMap[uint32(sps.ParameterID)] = sps
				}
			}
			for _, p := range stsd.AvcX.AvcC.PPSnalus {
				if pps, err := avc.ParsePPSNALUnit(p, spsMap); err == nil {
					ppsMap[pps.PicParameterSetID] = pps
				}
			}
		}
	}

	fmt.Println("=== slice headers near seam (frame_num / poc / idr_pic_id) ===")
	fragIdx := 0
	sampIdx := 0
	for _, seg := range parsed.Segments {
		for _, fr := range seg.Fragments {
			fs, err := fr.GetFullSamples(trex)
			if err != nil {
				fragIdx++
				continue
			}
			for _, s := range fs {
				dts := int64(s.DecodeTime)
				if dts < 6800 || dts > 9400 {
					sampIdx++
					continue
				}
				for _, nal := range splitAVCC(s.Data) {
					t := nal[0] & 0x1f
					if t == 1 || t == 5 { // non-IDR or IDR slice
						sh, err := avc.ParseSliceHeader(nal, spsMap, ppsMap)
						if err != nil {
							fmt.Printf("  s%04d frag%d dts=%d nalType=%d sliceHeader ERR: %v\n", sampIdx, fragIdx, dts, t, err)
							break
						}
						fmt.Printf("  s%04d frag%d dts=%d nalType=%d sliceType=%v frameNum=%d idrPicId=%d pocLsb=%d\n",
							sampIdx, fragIdx, dts, t, sh.SliceType, sh.FrameNum, sh.IDRPicID, sh.PicOrderCntLsb)
						break
					}
				}
				sampIdx++
			}
			fragIdx++
		}
	}
}

// splitAVCC splits a length-prefixed (4-byte) AVCC buffer into NAL units.
func splitAVCC(b []byte) [][]byte {
	var out [][]byte
	i := 0
	for i+4 <= len(b) {
		n := int(uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3]))
		i += 4
		if n <= 0 || i+n > len(b) {
			break
		}
		out = append(out, b[i:i+n])
		i += n
	}
	return out
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// nalTypes returns the list of H.264 NAL unit types present in an AVCC
// (length-prefixed) sample buffer.
func nalTypes(b []byte) []int {
	var out []int
	i := 0
	for i+4 <= len(b) {
		n := int(uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3]))
		i += 4
		if n <= 0 || i+n > len(b) {
			break
		}
		out = append(out, int(b[i]&0x1f))
		i += n
	}
	return out
}

// nalsByType returns the raw NAL payloads (without length prefix) of the given
// type from an AVCC (length-prefixed) sample buffer.
func nalsByType(b []byte, want int) [][]byte {
	var out [][]byte
	i := 0
	for i+4 <= len(b) {
		n := int(uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3]))
		i += 4
		if n <= 0 || i+n > len(b) {
			break
		}
		if int(b[i]&0x1f) == want {
			nal := make([]byte, n)
			copy(nal, b[i:i+n])
			out = append(out, nal)
		}
		i += n
	}
	return out
}
