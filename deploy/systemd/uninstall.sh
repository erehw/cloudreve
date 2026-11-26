#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
    echo '请用 sudo bash uninstall.sh 移除服务。' >&2
    exit 1
fi

unit=cloudreve.service
unit_file="/etc/systemd/system/$unit"
if [[ -f "$unit_file" ]]; then
    systemctl disable --now "$unit"
    rm -- "$unit_file"
    systemctl daemon-reload
fi

echo '服务已移除，程序和数据仍保存在 /app/cloudreve。'
