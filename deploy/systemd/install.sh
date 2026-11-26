#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
    echo '请用 sudo bash install.sh 安装。' >&2
    exit 1
fi

package_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
app_dir=/app/cloudreve
unit=cloudreve.service
unit_dir=/etc/systemd/system

if [[ ! -f "$package_dir/cloudreve" ]]; then
    echo '安装包缺少 Linux 版 cloudreve 可执行文件。' >&2
    exit 1
fi

# 确认当前系统能连接 systemd，再写入系统目录。
systemctl show --property=Version --value >/dev/null

if ! getent group cloudreve >/dev/null; then
    groupadd --system cloudreve
fi
if ! getent passwd cloudreve >/dev/null; then
    useradd --system --gid cloudreve --home-dir "$app_dir" --shell "$(command -v nologin)" cloudreve
fi

install -d -m 0755 "$app_dir"
install -d -o cloudreve -g cloudreve -m 0750 "$app_dir/data" "$app_dir/temp"
if [[ ! -e "$app_dir/data/conf.ini" ]]; then
    install -o cloudreve -g cloudreve -m 0600 "$package_dir/conf.ini.example" "$app_dir/data/conf.ini"
fi

# 先写入临时文件，再用 rename 更新二进制，允许替换正在运行的版本。
staged_binary=$(mktemp "$app_dir/.cloudreve.XXXXXX")
trap 'rm -f -- "$staged_binary"' EXIT
install -m 0755 "$package_dir/cloudreve" "$staged_binary"
if [[ -f "$app_dir/cloudreve" ]]; then
    install -m 0755 "$app_dir/cloudreve" "$app_dir/cloudreve.previous"
fi
mv -fT -- "$staged_binary" "$app_dir/cloudreve"

install -m 0644 "$package_dir/$unit" "$unit_dir/$unit"
systemctl daemon-reload
systemctl enable "$unit"
systemctl restart "$unit"
systemctl --no-pager status "$unit"
