#!/bin/sh
# 從 /etc/resolv.conf 取得 Kubernetes CoreDNS 的 ClusterIP
export NGINX_RESOLVER=$(awk '/^nameserver/{print $2; exit}' /etc/resolv.conf)
echo "NGINX_RESOLVER set to: ${NGINX_RESOLVER}"