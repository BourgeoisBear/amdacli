/*
Copyright 2023 Jason Stewart. All rights reserved.
Use of this source code is governed by the GPL v2 license,
as specified in the ./LICENSE file.
*/

package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

type DoMode uint8

const (
	DM_GET DoMode = iota
	DM_SET
)

func (m DoMode) String() string {
	switch m {
	case DM_GET:
		return "getConfig"
	case DM_SET:
		return "setConfig"
	}
	return ""
}

type Sess struct {
	url.URL
}

func URL2Sess(szUrl string) (Sess, error) {

	szUrl = strings.TrimSpace(szUrl)
	if len(szUrl) == 0 {
		return Sess{}, errors.New("empty URL")
	}

	// set proto to HTTP if unspecified
	if !strings.HasPrefix(szUrl, "http://") &&
		!strings.HasPrefix(szUrl, "https://") {
		szUrl = "http://" + szUrl
	}

	pUrl, err := url.Parse(szUrl)
	if err != nil {
		return Sess{}, err
	}

	return Sess{URL: *pUrl}, nil
}

func NewRq(szURL string) (*http.Request, error) {
	return http.NewRequest("GET", szURL, nil)
}

func ScrubKey(key string) string {
	prefixes := []string{"table.All.", "table."}
	for _, pf := range prefixes {
		if strings.HasPrefix(key, pf) {
			return strings.TrimPrefix(key, pf)
		}
	}
	return key
}

type KV struct {
	K, V string
}

func (kv *KV) Trim() {
	kv.K = strings.TrimSpace(kv.K)
	kv.V = strings.TrimSpace(kv.V)
}

func ToKV(s string) (ret KV, doSet bool) {
	ret.K, ret.V, doSet = strings.Cut(s, "=")
	ret.Trim()
	return
}

func ToUrl(v []KV) string {
	tmp := make([]string, 0, len(v))
	for _, kv := range v {
		if len(kv.K) > 0 {
			tmp = append(tmp, url.QueryEscape(kv.K)+"="+url.QueryEscape(kv.V))
		}
	}
	return strings.Join(tmp, "&")
}

func AppendParams(url string, params []KV) string {
	extra := ToUrl(params)
	if len(extra) > 0 {
		return url + "&" + extra
	}
	return url
}

func UrlConfigMgr(szHost string, action DoMode, params []KV) string {
	url := fmt.Sprintf(
		"%s/cgi-bin/configManager.cgi?action=%s",
		szHost,
		action.String(),
	)
	return AppendParams(url, params)
}

func Cmd2Url(host, cmd string) (string, error) {
	if strings.HasPrefix(cmd, "/") {
		return host + cmd, nil
	}

	kv, doSet := ToKV(cmd)
	if len(kv.K) == 0 {
		return "", errors.New("empty key")
	}

	if doSet {
		return UrlConfigMgr(host, DM_SET, []KV{{K: ScrubKey(kv.K), V: kv.V}}), nil
	} else {
		return UrlConfigMgr(host, DM_GET, []KV{{K: "name", V: ScrubKey(kv.K)}}), nil
	}
}

func (se *Sess) DoCmd(cmd string, prependHost bool) {

	host := se.URL.String()

	fnReport := func(s ...interface{}) {
		tmp := make([]interface{}, 0, len(s)+2)
		tmp = append(tmp, "\x1b[91;1mERR:\x1b[0m", host)
		for _, v := range s {
			tmp = append(tmp, v)
		}
		fmt.Fprintln(STDERR, tmp...)
	}

	url, err := Cmd2Url(host, cmd)
	if err != nil {
		fnReport("CMD", err)
		return
	}

	// report request URL
	fmt.Fprintln(STDERR, "\x1b[93;1mGET:\x1b[0m", url)

	pRq, err := NewRq(url)
	if err != nil {
		fnReport("URL2RQ", err)
		return
	}

	rsp, err := DigestAuthGet(pRq)
	if err != nil {
		fnReport("HTTP", err)
		return
	}

	// report non-200 responses
	if rsp.StatusCode != 200 {
		fnReport(fmt.Sprintf("RSP %d", rsp.StatusCode))
		return
	}

	// prepend host
	var pph []byte
	if prependHost {
		pph = append([]byte(se.URL.Host), '\t')
	}
	err = CopyBody(STDOUT, rsp, pph)
	if err != nil {
		fnReport("IO", err)
		return
	}
}

func CopyBody(dst io.Writer, rsp *http.Response, linePrefix []byte) error {
	if (rsp == nil) || (rsp.Body == nil) {
		return nil
	}
	defer rsp.Body.Close()

	iRdr := bufio.NewReader(rsp.Body)
	if rsp.Header.Get("Content-Encoding") == "gzip" {
		gzr, err := gzip.NewReader(rsp.Body)
		if err != nil {
			return err
		}
		defer gzr.Close()
		iRdr = bufio.NewReader(gzr)
	}

	for {
		ln, bpfx, err := iRdr.ReadLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if _, err = dst.Write(linePrefix); err != nil {
			return err
		}
		if _, err = dst.Write(ln); err != nil {
			return err
		}
		if !bpfx {
			if _, err = dst.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}

	return nil
}

const HELP_USAGE = `USAGE
  %[1]s [OPTION].. HOSTS [COMMAND]...

Batch API access to Amcrest & Dahua IP cameras.

If COMMAND(s) are given, runs each command against each HOST (i.e. camera),
then terminates. If no commands are provided, starts in interactive mode, where
commands can be supplied from the console.  Ctrl-d exits interactive mode.

OPTION
`

const HELP_REST = `
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
    Example: /cgi-bin/global.cgi?action=setCurrentTime&time=2011-7-3%%2021:02:32

PUTTING IT TOGETHER
  Interactive Mode:
    %[1]s 'user:userpass@mycam'

  Command Mode:
    %[1]s 'user:userpass@mycam' 'Multicast.TS[0]' 'AlarmServer.Enable=false'
`

var STDOUT, STDERR io.Writer

func main() {

	STDOUT, STDERR = os.Stdout, os.Stderr

	var err error

	defer func() {
		if err != nil {
			fmt.Fprintln(STDERR, err.Error())
			os.Exit(1)
		}
	}()

	// flags/help
	bAlwaysPrependHost := false
	bForceColor := false
	flag.BoolVar(&bAlwaysPrependHost, "a", false, "always prepend hostname to results (even when there is only one host)")
	flag.BoolVar(&bForceColor, "c", false, "force colors (even in pipelines)")
	flag.CommandLine.SetOutput(STDOUT)
	flag.Usage = func() {
		appname := filepath.Base(os.Args[0])
		fmt.Fprintf(STDOUT, HELP_USAGE, appname)
		flag.PrintDefaults()
		fmt.Fprintf(STDOUT, HELP_REST, appname)
	}

	flag.Parse()

	// color handling
	fd := os.Stdout.Fd()
	if bForceColor || isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
		STDOUT = colorable.NewColorable(os.Stdout)
		STDERR = colorable.NewColorable(os.Stderr)
	} else {
		STDOUT = colorable.NewNonColorable(os.Stdout)
		STDERR = colorable.NewNonColorable(os.Stderr)
	}

	// parse first arg as hosts string
	args := flag.Args()
	hosts := make([]Sess, 0)
	if len(args) > 0 {

		szHosts := args[0]
		args = args[1:]

		// parse hosts
		for _, hostUrl := range strings.Split(szHosts, ",") {
			oS, e2 := URL2Sess(hostUrl)
			if err != nil {
				err = e2
				return
			}
			hosts = append(hosts, oS)
		}
	}

	if len(hosts) == 0 {
		err = errors.New("no host(s) specified")
		return
	}
	for _, u := range hosts {
		fmt.Fprintln(STDERR, "\x1b[96;1mHOST:\x1b[0m", u.String())
	}

	// process commands
	if len(args) == 0 {

		// read commands from STDIN
		rl, e2 := readline.New("> ")
		if e2 != nil {
			err = e2
			return
		}
		defer rl.Close()

		for {
			cmd, e2 := rl.Readline()
			if e2 != nil {
				err = e2
				return
			}
			for _, host := range hosts {
				host.DoCmd(cmd, bAlwaysPrependHost || len(hosts) > 1)
			}
		}

	} else {

		// commands already in args
		for _, cmd := range args {
			for _, host := range hosts {
				host.DoCmd(cmd, bAlwaysPrependHost || len(hosts) > 1)
			}
		}
	}
}
