[English Version](README.md)

<h1 align="center">
  <br>
  <a href="https://cloudreve.org/" alt="logo" ><img src="https://raw.githubusercontent.com/cloudreve/frontend/master/public/static/img/logo192.png" width="150"/></a>
  <br>
  Cloudreve
  <br>
</h1>

<h4 align="center">支持多家云存储驱动的公有云文件系统.</h4>

<p align="center">
  <a href="https://dev.azure.com/abslantliu/cloudreve/_build?definitionId=6">
    <img src="https://img.shields.io/github/check-runs/cloudreve/cloudreve/master"
         alt="Azure pipelines">
  </a>
  <a href="https://github.com/cloudreve/cloudreve/releases">
    <img src="https://img.shields.io/github/v/release/cloudreve/cloudreve?include_prereleases" />
  </a>
  <a href="https://github.com/cloudreve/cloudreve/releases">
     <img src="https://badgen.net/static/release%20size/34%20MB/blue"/>
  </a>
  <a href="https://hub.docker.com/r/cloudreve/cloudreve">
  <img alt="Docker Pulls" src="https://img.shields.io/docker/pulls/cloudreve/cloudreve" />
  </a>
</p>
<p align="center">
  <a href="https://cloudreve.org">主页</a> •
  <a href="https://demo.cloudreve.org">演示</a> •
  <a href="https://github.com/cloudreve/cloudreve/discussions">讨论</a> •
  <a href="https://docs.cloudreve.org">文档</a> •
  <a href="https://github.com/cloudreve/cloudreve/releases">下载</a> •
  <a href="https://t.me/cloudreve_official">Telegram</a> •
  <a href="https://discord.com/invite/WTpMFpZT76">Discord</a>
</p>

## Cloudreve 自建版

- **团队文档。** 登录用户都能创建团队。团队管理员可以发邀请链接，每条最多邀请 10 人。成员可以共用文件夹、管理文件和查看历史版本。
- **邀请码注册。** 站点开启注册后，新用户需要使用站点管理员发放的邀请码。管理员可自动生成、自定义和更换邀请码。每个码默认可注册 10 个账号。
- **管理员和密码登录。** 空数据库首次启动时会随机生成管理员邮箱和密码，并写入 `./data/管理员密码.txt`。同一账号连续输错 10 次密码后，密码登录锁定 3 分钟。
- **Pro 功能显示。** 社区版默认隐藏 Pro 专属入口。站点管理员可在「后台 → 参数设置 → 站点信息」中调整「显示 Pro 功能」开关。

![Screenshot](https://raw.githubusercontent.com/cloudreve/docs/master/images/homepage.png)

## :sparkles: 特性

- :cloud: 支持本机、从机、七牛 Kodo、阿里云 OSS、腾讯云 COS、华为云 OBS、金山云 KS3、又拍云、OneDrive (包括世纪互联版) 、S3 兼容协议 作为存储端
- :outbox_tray: 上传/下载 支持客户端直传，支持下载限速
- 💾 可对接 Aria2/qBittorrent 离线下载，可使用多个从机节点分担下载任务
- 📚 在线 压缩/解压缩/压缩包预览、多文件打包下载
- 💻 覆盖全部存储策略的 WebDAV 协议支持
- :zap: 拖拽上传、目录上传、并行分片上传
- :card_file_box: 提取媒体元数据，通过元数据或标签搜索文件
- :family_woman_girl_boy: 多用户、用户组、多存储策略
- :link: 创建文件、目录的分享链接，可设定自动过期
- :eye_speech_bubble: 视频、图像、音频、 ePub 在线预览，文本、Office 文档在线编辑
- :art: 自定义配色、黑暗模式、PWA 应用、全站单页应用、国际化支持
- :rocket: All-in-One 打包，开箱即用
- 🌈 ... ...

## :hammer_and_wrench: 部署

Linux 部署包使用 `cloudreve.service`，新安装默认监听 `16003`。选择对应 CPU 架构的部署包，解压后运行 `sudo bash install.sh`，默认安装到 `/app/cloudreve`。详见 [systemd 安装、升级和 Caddy HTTPS 反代](deploy/systemd/README.md)。

你可以参考 [快速开始](https://docs.cloudreve.org/overview/quickstart) 启动一个本地实例进行体验、测试。

当你准备好将 Cloudreve 部署到生产环境时，可以参考 [部署](https://docs.cloudreve.org/overview/deploy/) 进行完整部署。

## :gear: 构建

在 Windows 上安装 Git、Go 1.26 或以上版本，以及带 npm 的 Node.js。在仓库根目录运行 `.\build.cmd`，即可生成 Windows x64 EXE 和 Linux x64 systemd 安装包。脚本会构建前端并嵌入两个程序，产物放在 Git 已忽略的 `release/` 目录。

你可以参考 [构建](https://docs.cloudreve.org/overview/build/) 从源代码构建 Cloudreve。

## :rocket: 贡献

如果你有兴趣为 Cloudreve 贡献代码，请参考 [贡献](https://docs.cloudreve.org/api/contributing/) 了解如何贡献。

## :alembic: 技术栈

- [Go](https://golang.org/) + [Gin](https://github.com/gin-gonic/gin) + [ent](https://github.com/ent/ent)
- [React](https://github.com/facebook/react) + [Redux](https://github.com/reduxjs/redux) + [Material-UI](https://github.com/mui-org/material-ui)

## :scroll: 许可证

GPL V3
