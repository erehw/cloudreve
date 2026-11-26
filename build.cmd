@echo off
setlocal EnableExtensions EnableDelayedExpansion
pushd "%~dp0" || exit /b 1

set "VERSION=4.19.2-selfhosted.1"
set "WORK=release/build"
set "ASSETS_ZIP=!WORK!/assets.zip"
set "WINDOWS_EXE=cloudreve-!VERSION!-windows-amd64.exe"
set "LINUX_PACKAGE=cloudreve-!VERSION!-linux-amd64-systemd"
set "TAR=%SystemRoot%\System32\tar.exe"

if not exist "assets\vite.config.ts" (
    git submodule update --init -- assets || goto :failed
)
if not exist "assets\node_modules\vite\bin\vite.js" (
    pushd assets || goto :failed
    set "HUSKY=0"
    call npm.cmd exec --yes --package=yarn@1.22.22 -- yarn install --frozen-lockfile --non-interactive
    if errorlevel 1 (
        popd
        goto :failed
    )
    popd
)

if not exist "!WORK!" mkdir "release\build" || goto :failed
set "NODE_OPTIONS=--max-old-space-size=8192"
set "npm_package_name=cloudreve-frontend"
set "npm_package_version=!VERSION!"
pushd assets || goto :failed
node node_modules/vite/bin/vite.js build --outDir "../!WORK!/assets/build" --emptyOutDir
if errorlevel 1 (
    popd
    goto :failed
)
popd
"!TAR!" -acf "!ASSETS_ZIP!" -C "!WORK!" assets/build || goto :failed
move /y "release\build\assets.zip" "application\statics\assets.zip" >nul || goto :failed

for /f %%G in ('git rev-parse --short HEAD') do set "COMMIT=%%G"
if not defined COMMIT goto :failed
set "CONSTANTS=github.com/cloudreve/Cloudreve/v4/application/constants"
set "LDFLAGS=-s -w -X !CONSTANTS!.BackendVersion=!VERSION! -X !CONSTANTS!.LastCommit=!COMMIT!"
set "CGO_ENABLED=0"
set "GOARCH=amd64"
set "GOAMD64=v1"
set "GOOS=windows"
go build -trimpath -ldflags "!LDFLAGS!" -o "!WORK!/!WINDOWS_EXE!" . || goto :failed
set "GOOS=linux"
go build -trimpath -ldflags "!LDFLAGS!" -o "!WORK!/cloudreve" . || goto :failed

rem Assign Linux executable permissions explicitly when packaging from NTFS.
(
    echo #mtree
    echo ./!LINUX_PACKAGE! type=dir mode=0755 uid=0 gid=0
    echo ./!LINUX_PACKAGE!/cloudreve type=file mode=0755 uid=0 gid=0 contents=!WORK!/cloudreve
    for %%F in (install.sh uninstall.sh) do echo ./!LINUX_PACKAGE!/%%F type=file mode=0755 uid=0 gid=0 contents=deploy/systemd/%%F
    for %%F in (cloudreve.service conf.ini.example Caddyfile.example README.md) do echo ./!LINUX_PACKAGE!/%%F type=file mode=0644 uid=0 gid=0 contents=deploy/systemd/%%F
    echo ./!LINUX_PACKAGE!/LICENSE type=file mode=0644 uid=0 gid=0 contents=LICENSE
) > "!WORK!/linux-package.mtree" || goto :failed
"!TAR!" -czf "!WORK!/!LINUX_PACKAGE!.tar.gz" "@!WORK!/linux-package.mtree" || goto :failed

move /y "release\build\!WINDOWS_EXE!" "release\!WINDOWS_EXE!" >nul || goto :failed
move /y "release\build\!LINUX_PACKAGE!.tar.gz" "release\!LINUX_PACKAGE!.tar.gz" >nul || goto :failed
echo release\!WINDOWS_EXE!
echo release\!LINUX_PACKAGE!.tar.gz
popd
exit /b 0

:failed
>&2 echo Cloudreve build failed.
popd
exit /b 1
