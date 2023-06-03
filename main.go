package main

import (
	"bufio"
	"compress/gzip"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
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

	// set proto to HTTP if unspecified
	szUrl = strings.TrimSpace(szUrl)
	if len(szUrl) > 0 &&
		!strings.HasPrefix(szUrl, "http://") &&
		!strings.HasPrefix(szUrl, "https://") {
		szUrl = "http://" + szUrl
	}

	pUrl, err := url.Parse(szUrl)
	if err != nil {
		return Sess{}, err
	}

	return Sess{URL: *pUrl}, nil
}

func GenNonce64() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func GenAuthMD5(parts ...string) string {
	vmd5 := md5.Sum([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(vmd5[:])
}

func SplitWwwAuthenticate(wwwauth string) map[string]string {
	parts := strings.Split(wwwauth, ",")
	RET := make(map[string]string, len(parts))
	for _, pt := range parts {
		pt = strings.TrimSpace(pt)
		sKv := strings.Split(pt, "=")
		if len(sKv) > 1 {
			if val, err := strconv.Unquote(sKv[1]); err == nil {
				RET[sKv[0]] = val
			}
		}
	}
	return RET
}

func GenDigestAuth(rq *http.Request, szWwwAuth string) (string, error) {

	var szUser, szPass string
	if rq.URL.User != nil {
		szUser = rq.URL.User.Username()
		szPass, _ = rq.URL.User.Password()
	}

	pt := SplitWwwAuthenticate(szWwwAuth)
	if len(pt) == 0 {
		return "", errors.New("empty WWW-Authenticate map")
	}

	cnonce, err := GenNonce64()
	if err != nil {
		return "", err
	}

	// generate hashes
	URI := rq.URL.RequestURI()
	ha1 := GenAuthMD5(szUser, pt["Digest realm"], szPass)
	ha2 := GenAuthMD5(rq.Method, URI)
	response := GenAuthMD5(ha1, pt["nonce"], "1", cnonce, pt["qop"], ha2)

	// quoting & order
	mAuth := [][]string{
		[]string{"Digest username", szUser},
		[]string{"realm", pt["Digest realm"]},
		[]string{"nonce", pt["nonce"]},
		[]string{"uri", URI},
		[]string{"cnonce", cnonce},
		[]string{"nc", "1"},
		[]string{"qop", pt["qop"]},
		[]string{"response", response},
	}
	parts := make([]string, 0, len(mAuth))
	for _, v := range mAuth {
		parts = append(parts, v[0]+"="+strconv.Quote(v[1]))
	}
	return strings.Join(parts, ", "), nil
}

func NewRq(szURL string) (*http.Request, error) {
	return http.NewRequest("GET", szURL, nil)
}

/*
HTTP GET w/ Digest Access Authentication
https://en.wikipedia.org/wiki/Digest_access_authentication
NOTE: pre-closes response body on non-200 response status
*/
func DigestAuthGet(pRq *http.Request) (rsp *http.Response, err error) {

	rsp, err = http.DefaultClient.Do(pRq)
	if err != nil {
		return
	}

	// retry w/ auth
	if rsp.StatusCode == 401 {
		rsp.Body.Close()
		rsp.Body = nil
		wwwAuth := rsp.Header.Get("Www-Authenticate")
		szAuth, e2 := GenDigestAuth(pRq, wwwAuth)
		if e2 != nil {
			err = fmt.Errorf("%w, %s", e2, wwwAuth)
			return
		}
		pRq.Header.Set("Authorization", szAuth)

		rsp, err = http.DefaultClient.Do(pRq)
		if err != nil {
			return
		}
	}

	// err on non-200
	if rsp.StatusCode != 200 {
		rsp.Body.Close()
		rsp.Body = nil
	}
	return
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
		// TODO: windows cli colors
		// TODO: isatty
		tmp := make([]interface{}, 0, len(s)+2)
		tmp = append(tmp, "\x1b[91m"+"ERR "+"\x1b[0m", host)
		for _, v := range s {
			tmp = append(tmp, v)
		}
		fmt.Fprintln(os.Stderr, tmp...)
	}

	url, err := Cmd2Url(host, cmd)
	if err != nil {
		fnReport("CMD", err)
		return
	}

	// report request URL
	fmt.Println("\x1b[93m" + url + "\x1b[0m")

	pRq, err := NewRq(url)
	if err != nil {
		fnReport("URL2RQ", err)
		return
	}

	rsp, err := DigestAuthGet(pRq)
	if err != nil {
		fnReport("GET", err)
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
	err = CopyBody(os.Stdout, rsp, pph)
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

func main() {

	var err error
	defer func() {
		if err != nil {
			// TODO: color
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}()

	// flags/help
	bAlwaysPrependHost := false
	flag.BoolVar(&bAlwaysPrependHost, "a", false, "always prepend hostname to results\n(i.e. even when there is only one host)")
	flag.CommandLine.SetOutput(os.Stdout)
	flag.Usage = func() {
		iWri := os.Stdout
		fmt.Fprint(iWri, `USAGE
  camcli [OPTION].. HOSTS [COMMAND]...

Batch API access to Amcrest & Dahua IP cameras.

If COMMAND(s) are given, runs each command against each HOST (i.e. camera),
then terminates. If no commands are provided, starts in interactive mode, where
commands can be supplied from the console.

OPTION
`)

		flag.PrintDefaults()

		fmt.Fprint(iWri, `
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
    NOTE: Does not URL-encode parameters like other commands.
          URL parameters must be encoded manually.
    Example: /cgi-bin/global.cgi?action=setCurrentTime&time=2011-7-3%2021:02:32

PUTTING IT TOGETHER
  Interactive Mode:
    camcli 'user:userpass@mycam'

  Command Mode:
    camcli 'user:userpass@mycam' 'Multicast.TS[0]' 'AlarmServer.Enable=false'
`)
	}

	flag.Parse()
	args := flag.Args()
	hosts := make([]Sess, 0)

	// parse first arg as hosts string
	if len(args) > 0 {

		szHosts := args[0]
		args = args[1:]

		// parse hosts
		htmp := strings.Split(szHosts, ",")
		for ix := range htmp {
			oS, e2 := URL2Sess(htmp[ix])
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
		fmt.Println(u.String())
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
