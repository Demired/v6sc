#!/bin/sh

myfile="/root"
if [ ! -d $myfile ]; then
    go build&&./v6sc --port=8000
else
    nohup ./v6sc >/dev/null 2>&1 &
fi