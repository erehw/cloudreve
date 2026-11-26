[中文版本](README_zh-CN.md)

<h1 align="center">
  <br>
  <a href="https://cloudreve.org/" alt="logo" ><img src="https://raw.githubusercontent.com/cloudreve/frontend/master/public/static/img/logo192.png" width="150"/></a>
  <br>
  Cloudreve
  <br>
</h1>
<h4 align="center">Self-hosted file management system with multi-cloud support.</h4>

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
  <a href="https://cloudreve.org">Homepage</a> •
  <a href="https://demo.cloudreve.org">Try it</a> •
  <a href="https://github.com/cloudreve/cloudreve/discussions">Discussion</a> •
  <a href="https://docs.cloudreve.org">Documents</a> •
  <a href="https://github.com/cloudreve/cloudreve/releases">Download</a> •
  <a href="https://t.me/cloudreve_official">Telegram</a> •
  <a href="https://discord.com/invite/WTpMFpZT76">Discord</a>
</p>

## Cloudreve self-hosted edition

- **Team documents.** Any signed-in user can create a team. Team administrators can invite up to 10 people per link. Members share folders, manage files, and view file versions.
- **Invite-only registration.** When registration is enabled, new users need a code issued by a site administrator. Administrators can generate, customize, or replace a code. Each code defaults to 10 registrations.
- **Administrator setup and sign-in.** The first start with an empty database generates an email address and password for the administrator and saves both to `./data/管理员密码.txt`. Ten consecutive password failures lock password sign-in for that account for three minutes.
- **Pro feature visibility.** The community edition hides Pro-only entries by default. Site administrators can change their visibility under **Settings → Site Information**.

![Screenshot](https://raw.githubusercontent.com/cloudreve/docs/master/images/homepage.png)

## :sparkles: Features

- :cloud: Support storing files into Local, Remote node, OneDrive, S3 compatible API, Qiniu Kodo, Aliyun OSS, Tencent COS, Huawei Cloud OBS, Kingsoft Cloud KS3, Upyun.
- :outbox_tray: Upload/Download in directly transmission from client to storage providers.
- 💾 Integrate with Aria2/qBittorrent to download files in background, use multiple download nodes to share the load.
- 📚 Compress/Extract/Preview archived files, download files in batch.
- 💻 WebDAV support covering all storage providers.
- :zap:Drag&Drop to upload files or folders, with parallel resumable upload support.
- :card_file_box: Extract media metadata from files, search files by metadata or tags.
- :family_woman_girl_boy: Multi-users with multi-groups.
- :link: Create share links for files and folders with expiration date.
- :eye_speech_bubble: Preview videos, images, audios, ePub files online; edit texts, diagrams, Markdown, images, Office documents online.
- :art: Customize theme colors, dark mode, PWA application, SPA, i18n.
- :rocket: All-in-one packaging, with all features out of the box.
- 🌈 ... ...

## :hammer_and_wrench: Deploy

The Linux deployment package uses `cloudreve.service` and listens on port `16003` on a fresh install. Extract the package for your CPU architecture and run `sudo bash install.sh` from the extracted directory. The default installation path is `/app/cloudreve`. See the [systemd and Caddy deployment guide (Chinese)](deploy/systemd/README.md).

To deploy Cloudreve, you can refer to [Getting started](https://docs.cloudreve.org/overview/quickstart) for a quick local deployment to test.

When you're ready to deploy Cloudreve to a production environment, you can refer to [Deploy](https://docs.cloudreve.org/overview/deploy/) for a complete deployment.

## :gear: Build

On Windows, install Git, Go 1.26 or later, and Node.js with npm. Run `.\build.cmd` from the repository root to produce the Windows x64 EXE and Linux x64 systemd archive under `release/`. The script builds and embeds the frontend in both binaries. Git ignores `release/`.

Please refer to [Build](https://docs.cloudreve.org/overview/build/) for how to build Cloudreve from source code.

## :rocket: Contributing

If you're interested in contributing to Cloudreve, please refer to [Contributing](https://docs.cloudreve.org/api/contributing/) for how to contribute to Cloudreve.

## :alembic: Stacks

- [Go](https://golang.org/) + [Gin](https://github.com/gin-gonic/gin) + [ent](https://github.com/ent/ent)
- [React](https://github.com/facebook/react) + [Redux](https://github.com/reduxjs/redux) + [Material-UI](https://github.com/mui-org/material-ui)

## :scroll: License

GPL V3
