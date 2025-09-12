package cookies

import (
	"encoding/json"
	"os"

	"github.com/go-rod/rod/lib/proto"
)

// ChromeCookie 定义 Chrome 导出 JSON 的结构
type ChromeCookie struct {
	Domain         string  `json:"domain"`
	ExpirationDate float64 `json:"expirationDate,omitempty"`
	HostOnly       bool    `json:"hostOnly"`
	HTTPOnly       bool    `json:"httpOnly"`
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	SameSite       string  `json:"sameSite"`
	Secure         bool    `json:"secure"`
	Session        bool    `json:"session"`
	Value          string  `json:"value"`
}

func Get() []*proto.NetworkCookieParam {
	data, err := os.ReadFile("./cookies.json")
	if err != nil {
		panic(err)
	}

	var chromeCookies []ChromeCookie
	if err := json.Unmarshal(data, &chromeCookies); err != nil {
		panic(err)
	}
	var rodCookies []*proto.NetworkCookieParam
	for _, c := range chromeCookies {
		rodCookie := &proto.NetworkCookieParam{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
		}

		rodCookies = append(rodCookies, rodCookie)
	}
	return rodCookies
}
