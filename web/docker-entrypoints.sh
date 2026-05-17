#!/bin/sh
# This script runs as 40-resolver.sh, AFTER nginx's 20-envsubst-on-templates.sh.
# Each script runs in its own subshell, so we cannot export NGINX_RESOLVER
# and have it picked up by envsubst. Instead, we sed-replace the literal
# "${NGINX_RESOLVER}" that envsubst left in the already-rendered default.conf.

NGINX_RESOLVER=$(awk '/^nameserver/{print $2; exit}' /etc/resolv.conf)
echo "NGINX_RESOLVER set to: ${NGINX_RESOLVER}"

CONF=/etc/nginx/conf.d/default.conf
if [ -f "$CONF" ]; then
    sed -i "s|\${NGINX_RESOLVER}|${NGINX_RESOLVER}|g" "$CONF"
    echo "Patched resolver in ${CONF}"
else
    echo "WARNING: ${CONF} not found, resolver not patched"
fi