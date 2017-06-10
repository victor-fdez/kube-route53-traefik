#!/usr/bin/env bash


echo "Building victor755555/kube-traefik:build"

docker build -t victor755555/kube-traefik:build . -f Dockerfile.build

docker create --name extract victor755555/kube-traefik:build  
docker cp extract:/go/src/github.com/victor-fdez/kube-route53-traefik/kube-traefik ./kube-traefik  
docker rm -f extract

echo "Building victor755555/kube-traefik:latest"

docker build --no-cache -t victor755555/kube-traefik:latest .
rm ./kube-traefik
