#!/bin/sh

if [[ "$(cat /root/.bashrc | egrep -i 'MYSQL_PASSWORD')" != "" ]] && [[ "$(env | egrep -i 'MYSQL_PASSWORD')" != "" ]];then
    echo "已经设置过环境变量"
else
    export MYSQL_PASSWORD=`openssl rand -base64 30`
    echo "export $(env | egrep -i 'MYSQL_PASSWORD')" >> /root/.bashrc
    echo "初始化完成"
fi