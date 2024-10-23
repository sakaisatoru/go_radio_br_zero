package netradio

import (
	"fmt"
	"context"
	"os"
	"github.com/carlmjohnson/requests"
	"net/http"
	"encoding/base64"
	"strconv"
	"regexp"
	"strings"
)

type StationInfo struct {
	Name string
	Url string
}

const (
	auth_key string = "bcd151073c03b352e1ef2fd66c32209da9ca0afa" // 現状は固有 key_lenght = 0
	tokenfile string = "/run/radiko_token"
)

func gen_temp_chunk_m3u8_url(url string, auth_token string) (string, error) {
	var (
		chunkurl string
		err error
	)
	
    headers := make(http.Header)
    headers.Add("X-Radiko-AuthToken", auth_token)
	
	h2 := http.Header{}
	var s string
	err = requests.
			URL(url).
			Headers(headers).
			CopyHeaders(h2).
			ToString(&s).
			Fetch(context.Background())
	if err == nil {
		re := regexp.MustCompile(`https?://.+m3u8`)
		chunkurl = re.FindString(s)
	} else {
		chunkurl = ""
	}
	return chunkurl, err
}

func AFN_get_url_with_api(station string) (string, error) {
	url := fmt.Sprintf("https://playerservices.streamtheworld.com/api/livestream?station=%s&transports=http,hls&version=1.8", station)
	var s string
	err := requests.
			URL(url).
			ToString(&s).
			Fetch(context.Background())
	var u string
	if err == nil {
		re := regexp.MustCompile("<ip>(.+?)</ip>")
		m := re.FindStringSubmatch(s)
		if len(m) > 0 {
			u = fmt.Sprintf("http://%s/%s.mp3", string(m[1]), station)
		} else {
			u = ""
		}
	} else {
		//~ fmt.Println("afn api ",err)
		u = ""
	}
	return u, err
}

func Radiko_get_url(station string) (string, error) {
	var (
		authtoken string
		chunkurl string
		err error = nil
	)

	station_url := fmt.Sprintf("http://f-radiko.smartstream.ne.jp/%s/_definst_/simul-stream.stream/playlist.m3u8", station)
	
	t, _ := os.ReadFile(tokenfile)
	authtoken = string(t)
	chunkurl, err = gen_temp_chunk_m3u8_url(station_url, authtoken)
		
	if err != nil || len(chunkurl) == 0 {
		url := "https://radiko.jp/v2/api/auth1"
		
		h := make(http.Header)
		h.Add("User-Agent", "curl/7.56.1")
		h.Add("Accept", "*/*")
		h.Add("X-Radiko-App", "pc_html5")
		h.Add("X-Radiko-App-Version", "0.0.1")
		h.Add("X-Radiko-User", "dummy_user")
		h.Add("X-Radiko-Device", "pc")

		h2 := http.Header{}
		var s string
		err := requests.
				URL(url).
				Headers(h).
				CopyHeaders(h2).
				ToString(&s).
				Fetch(context.Background())
		if err != nil {
			//~ fmt.Println("Error ",err)
			goto exit_this
		}
	
		authtoken  = h2.Get("x-radiko-authtoken")
		offset, _  := strconv.Atoi(h2.Get("x-radiko-keyoffset"))
		length, _  := strconv.Atoi(h2.Get("x-radiko-keylength"))
		partialkey := base64.StdEncoding.EncodeToString([]byte(auth_key[offset:offset+length]))
		
		//~ fmt.Println("authtoken update.")
		os.WriteFile(tokenfile, []byte(authtoken), 0666)

		url2 := "https://radiko.jp/v2/api/auth2"
		h3 := make(http.Header)
		h3.Add("X-Radiko-AuthToken", authtoken)
		h3.Add("X-Radiko-Partialkey", partialkey)
		h3.Add("X-Radiko-User", "dummy_user")
		h3.Add("X-Radiko-Device", "pc")

		h4 := http.Header{}
		var ss string			// ss にはリージョンが入る
		err = requests.
				URL(url2).
				Headers(h3).
				CopyHeaders(h4).
				ToString(&ss).
				Fetch(context.Background())
		if err != nil {
			//~ fmt.Println("Error ",err)
			goto exit_this
		}
		chunkurl, err = gen_temp_chunk_m3u8_url(station_url, authtoken)
	}
exit_this:
	return chunkurl, err
}

func Radiko_setup(stlist []*StationInfo) {
	for _, st := range stlist {
		args := strings.Split(st.Url, "/")
		if args[0] == "plugin:" {
			if args[1] == "radiko.py" {
				_, _ = Radiko_get_url(args[2])
				break
			}
		}
	}
}
