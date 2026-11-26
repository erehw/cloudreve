# 在 Linux 上部署 Cloudreve

在使用 systemd 的 Linux 服务器上，选择对应 CPU 架构的压缩包。常见的 Intel / AMD 服务器使用 `amd64`，ARM 服务器使用 `arm64`。可运行 `uname -m` 查看架构。

解压后，进入解压出的目录并运行安装脚本。

```bash
sudo bash install.sh
```

安装器把程序放到 `/app/cloudreve`，创建 `cloudreve` 系统账号，注册 `cloudreve.service`，并设置开机启动。新安装监听 16003，访问 `http://服务器IP:16003/`。从其他电脑访问时，服务器防火墙需要允许该端口。

配置文件在 `/app/cloudreve/data/conf.ini`。修改 `[System]` 下的 `Listen` 后，运行 `sudo systemctl restart cloudreve`。空数据库首次启动时会生成管理员邮箱和密码。从 `/app/cloudreve/data/管理员密码.txt` 读取这两项并登录。

```bash
sudo systemctl status cloudreve
sudo journalctl -u cloudreve -f
```

## 使用 Caddy 反代 HTTPS

以下步骤用于 Caddy 与 Cloudreve 运行在同一台服务器上的部署。

1. 把域名解析到服务器，并允许外部访问 TCP 80 和 443。Caddy 可以为公网域名自动签发及续期证书。[Caddy 自动 HTTPS](https://caddyserver.com/docs/automatic-https)
2. 编辑 `/app/cloudreve/data/conf.ini`，把 `[System]` 下的 `Listen` 改为 `127.0.0.1:16003`。运行 `sudo systemctl restart cloudreve`。
3. 按 Caddy 官方方式安装 Caddy，把下面的站点段加入 `/etc/caddy/Caddyfile`，并换成自己的域名。包内的 `Caddyfile.example` 可直接参考。[安装 Caddy](https://caddyserver.com/docs/install) · [Caddy 反向代理](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)

   ```caddyfile
   cloud.example.com {
       reverse_proxy 127.0.0.1:16003
   }
   ```

4. 验证配置并重新加载。Caddy 尚未启动时，运行 `sudo systemctl enable --now caddy` 启动即可。[使用 Caddy 系统服务](https://caddyserver.com/docs/running#using-the-service)

   ```bash
   sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
   sudo systemctl reload caddy
   ```

5. 通过 `https://cloud.example.com/` 登录 Cloudreve。在 **后台 → 参数设置 → 站点信息** 中，把站点 URL 改为实际的 HTTPS 地址。使用域名打开团队页面并生成邀请链接。

## 升级和移除

升级前备份 `/app/cloudreve/data` 及实际使用的数据库。解压新版本，重新运行 `sudo bash install.sh`。脚本保留原配置和数据，并把旧程序保存为 `/app/cloudreve/cloudreve.previous`。从手动安装迁移时，需保证 `cloudreve` 系统账号能读写实际的数据和存储目录。

运行 `sudo bash uninstall.sh` 会停止服务、取消开机启动并移除 systemd 单元。程序、配置和数据保留在 `/app/cloudreve`。
