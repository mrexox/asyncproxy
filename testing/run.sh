#!/usr/bin/env bash

if [ "$1" == "many" ] || [ "$1" == "" ]
then
    n=50000
    c=1000
else
    n=$1
    if [ "$2" != "" ]
    then
        c=$2
    else
        c=1
    fi
fi

ab -r -T 'application/xml' -p request.xml -m POST -n ${n} -c ${c} http://localhost:8080/notifications
