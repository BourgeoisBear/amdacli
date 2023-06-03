/*
Copyright 2023 Jason Stewart. All rights reserved.
Use of this source code is governed by the GPL v2 license,
as specified in the ./LICENSE file.
*/

package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

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
