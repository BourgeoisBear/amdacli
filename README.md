# amdacli: Amcrest / Dahua Command CLI

Run HTTP configuration commands across Amcrest or Dahua IP camera(s).

Interact with a single camera, or several in one command!

## Installation

1. Install the latest Go compiler from https://golang.org/dl/
2. Install the program:

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
