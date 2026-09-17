# Chores Kindle

Simple monochrome display app for chores/tasks on a Fire 7 tablet.

## Building

```bash
export CHORES_API_TOKEN=your_token_here
CGO_ENABLED=0 go build -ldflags="-s -w -buildid=" -trimpath -o chores-kindle main.go
```

The app compiles to a single binary with no dependencies.

## Running

```bash
./chores-kindle
```

- Default port: `8999`
- Override port: `./chores-kindle -port 9001`

The app self-discovers the family ID from the chores API on startup, so no additional configuration is needed.

## Deploying to kate

Download the latest `chores-kindle` binary from [Releases](https://github.com/gunnaringe/chores-kindle/releases/latest) and follow the steps in `chores-kindle.service` to deploy as a systemd unit on the k3s Ubuntu VM (`192.168.1.164`).

Fill in the `CHORES_API_TOKEN` environment variable in the unit before deploying.

## Accessing from Kindle

1. On the Fire tablet, open Silk browser
2. Navigate to: `http://192.168.1.164:8999`

## Keeping the screen awake

The app requests the Screen Wake Lock API, but for reliable always-on display, also configure the Kindle's OS settings:

**Manual Setup:**
1. Settings → Display & Sounds → Screensaver: set to "Off"
2. Settings → Display & Sounds → Sleep: set to "Never"
3. (Optional) Settings → WiFi → keep WiFi connected during sleep
