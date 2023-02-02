# Binary

Kerberos Agents are now also shipped as static binaries. Within the Docker image build, we are extracting the Kerberos Agent binary and are [uploading them to the releases page](https://github.com/kerberos-io/agent/releases) in the repository. By opening a release you'll find a `.tar` with the relevant files.

- `main`: this is the Kerberos Agent binary.
- `data`: the folder containing the recorded video, configuration, etc.
- `mp4fragment`: a binary to transform MP4s to Fragmented MP4s.
- `www`: the Kerberos Agent ui (compiled React app).

You can run the binary as following on port `8080`:

    main run cameraname 8080

## Systemd

When running on a Linux OS you might consider to auto-start the Kerberos Agent using systemd. Create a file called `/etc/systemd/system/kerberos-agent.service` and copy-paste following configuration. Update the `WorkingDirectory` and `ExecStart` accordingly.

    [Unit]
    Wants=network.target
    [Service]
    ExecStart=/home/pi/agent/main run camera 80
    WorkingDirectory=/home/pi/agent/
    [Install]
    WantedBy=multi-user.target

To load your new service, we'll execute following commands.

    sudo systemctl daemon-reload
    sudo systemctl enable kerberos-agent
    sudo systemctl start kerberos-agent

Confirm the service is running:

    sudo systemctl status kerberos-agent
