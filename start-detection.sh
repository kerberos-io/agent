#! /bin/bash

# Stop and start Docker containers
docker compose down --remove-orphans && docker compose up -d

# Check if Docker containers are up before continuing
if [ $? -ne 0 ]; then
    echo "Docker containers failed to start"
    exit 1
fi

# Run FFmpeg to stream video
ffmpeg -f v4l2 -input_format yuyv422 -video_size 640x480 -i /dev/video0 \
    -vcodec libx264 -preset ultrafast -tune zerolatency -f rtsp rtsp://localhost:8554/cam &

# Start mediamtx
./mediamtx

# wait for FFmpeg to finish before exiting the script
wait
