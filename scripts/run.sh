#!/bin/bash

autoremoval() {
  partition=/dev/
  imagedir=/opensource/data/recordings
  clouddir=/opensource/data/cloud

  while :
  do
    #find $imagedir -type f -mtime +2 -exec rm {} \;
    #find $clouddir -type f -mtime +2 -exec rm {} \;
    sleep 60
  done
}

export LD_LIBRARY_PATH=/lib:/usr/lib:/usr/local/lib && ldconfig

autoremoval &

/usr/bin/supervisord -n -c /etc/supervisord.conf
