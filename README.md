# amdacli: Amcrest / Dahua Command CLI

Run HTTP configuration commands across Amcrest or Dahua IP camera(s).

Interact with a single camera, or several in one command!

## Installation

1. Download the latest Go compiler from https://golang.org/dl/.
2. Follow the instructions for your operating system (https://go.dev/doc/install).
3. Install the program:

```sh
go install github.com/BourgeoisBear/amdacli@latest
```

## Help
```
USAGE
  amdacli [OPTION].. HOSTS [COMMAND]...

Batch API access to Amcrest & Dahua IP cameras.

If COMMAND(s) are given, runs each command against each HOST (i.e. camera),
then terminates. If no commands are provided, starts in interactive mode, where
commands can be supplied from the console.  Ctrl-d exits interactive mode.

OPTION
  -a    always prepend hostname to results (even when there is only one host)
  -c    force colors (even in pipelines)

HOSTS
  Comma-separated list of camera hosts to interact with, where each host takes
  the format 'http(s)://username:password@hostname'.  If the protocol is left
  unspecified, 'http://' is assumed.

  Example: "admin:mypass1@doorcam,https://admin:mypass2@192.168.1.50"

COMMAND
  PropertyName
    Get current value of PropertyName via configManager.cgi.
    Example: Multicast.TS[0]

  PropertyName=NewValue
    Set value of PropertyName to NewValue via configManager.cgi.
    Example: Multicast.TS[0].TTL=1

  /RequestURL
    Forward raw request URL to camera API.
    Does not URL-encode parameters like other commands.
    URL parameters must be encoded manually.
    Example: /cgi-bin/global.cgi?action=setCurrentTime&time=2011-7-3%2021:02:32

PUTTING IT TOGETHER
  Interactive Mode:
    amdacli 'user:userpass@mycam'

  Command Mode:
    amdacli 'user:userpass@mycam' 'Multicast.TS[0]' 'AlarmServer.Enable=false'

```

## Useful API Calls

### Query

| Action                          | Call                                                                  |
| ------                          | ----                                                                  |
| Fetch current settings          | `All`                                                                 |
| Get system info                 | `/cgi-bin/magicBox.cgi?action=getSystemInfo`                          |
| Get software version            | `/cgi-bin/magicBox.cgi?action=getSoftwareVersion`                     |
| Get device class                | `/cgi-bin/magicBox.cgi?action=getDeviceClass`                         |
| Get machine name                | `/cgi-bin/magicBox.cgi?action=getMachineName`                         |
| Get vendor                      | `/cgi-bin/magicBox.cgi?action=getVendor`                              |
| Get language capabilities       | `/cgi-bin/magicBox.cgi?action=getLanguageCaps`                        |
| Get current time                | `/cgi-bin/global.cgi?action=getCurrentTime`                           |
| Get encode config capability    | `/cgi-bin/encode.cgi?action=getConfigCaps[&channel=<chan_num>]`       |
| Get recording capability        | `/cgi-bin/recordManager.cgi?action=getCaps`                           |
| Get event management capability | `/cgi-bin/eventManager.cgi?action=getCaps`                            |
| Get PTZ capability              | `/cgi-bin/ptz.cgi?action=getCurrentProtocolCaps[&channel=<chan_num>]` |

### Alter

| Action           | Call                                                                 |
| ------           | ----                                                                 |
| Reboot           | `/cgi-bin/magicBox.cgi?action=reboot`                                |
| Shutdown         | `/cgi-bin/magicBox.cgi?action=shutdown`                              |
| Set current time | `/cgi-bin/global.cgi?action=setCurrentTime&time=YYYY-M-D%20H:m:s` |

