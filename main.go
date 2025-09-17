package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/simplifiedchinese"
	_ "modernc.org/sqlite"

	"soushu-go/cookies"
)

type Config struct {
	BaseUrl      string `json:"baseUrl"`
	PathUrl      string `json:"pathUrl"`
	Type         int8   `json:"type"`
	DownloadPath string `json:"downloadPath"`
	UseProxy     bool   `json:"useProxy"`
}

var currentProxy int
var useProxy bool

func main() {
	currentProxy = 0
	configRaw, err := os.ReadFile("./config.json")
	if err != nil {
		panic(err)
	}

	var config Config
	if err := json.Unmarshal(configRaw, &config); err != nil {
		panic(err)
	}
	useProxy = config.UseProxy

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s.db", config.DownloadPath))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	db.Exec(`
		CREATE TABLE IF NOT EXISTS novels (
			tid INTEGER PRIMARY KEY,
			title TEXT,
			content TEXT
		)
	`)

	l := launcher.New().
		Proxy("127.0.0.1:60000").
		MustLaunch()

	browser := rod.New().ControlURL(l).MustConnect()
	defer browser.MustClose()
	rodCookies := cookies.Get()
	browser.SetCookies(rodCookies)
	browser.MustPage()

	logFile, err := os.OpenFile("current.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println("Open Log File Error:")
		panic(err)
	}
	defer logFile.Close()

	currentMenuUrl := config.PathUrl
	for {
		if _, err := logFile.WriteString(currentMenuUrl + "\n"); err != nil {
			fmt.Println("Write Log Error")
			panic(err)
		}

		fmt.Println(config.BaseUrl + currentMenuUrl)
		menuPage := OpenValidPage(browser, config.BaseUrl+currentMenuUrl)

		tbodys, _ := menuPage.Elements("table#threadlisttableid tbody")

		for _, tbody := range tbodys {
			id, _ := tbody.Attribute("id")
			if id == nil {
				continue
			}
			text, _ := tbody.Text()

			if (config.Type == 0 && strings.Contains(*id, "normalthread")) ||
				(config.Type == 1 && strings.Contains(*id, "normalthread") && strings.Contains(text, "[已解决]")) {
				aElement, _ := tbody.Element("a.s.xst")
				href, _ := aElement.Attribute("href")

				if strings.Contains(*href, "adver") {
					continue
				}

				u, _ := url.Parse(*href)
				q := u.Query()
				tid := q.Get("tid")

				var exists bool
				err := db.QueryRow(`
					SELECT EXISTS(
						SELECT 1 FROM novels WHERE tid = ?
					)
				`, tid).Scan(&exists)
				if err != nil {
					panic(err)
				}

				if exists {
					fmt.Println("Exists")
				} else {
					threadPage := OpenValidPage(browser, config.BaseUrl+*href)
					if threadPage == nil {
						continue
					}

					titleElement, err := threadPage.Element("span#thread_subject")
					if err != nil {
						fmt.Println("Get Title Error", config.BaseUrl+*href)
						panic(err)
					}
					titleStr, _ := titleElement.Text()
					fmt.Println("Title: ", titleStr)

					posts, err := threadPage.Elements("div.pcb")
					if err != nil {
						fmt.Println("Get Post Error: ", config.BaseUrl+*href)
						panic(err)
					}

					var post *rod.Element
					if config.Type == 0 {
						post = posts[0]
					} else if config.Type == 1 {
						if len(posts) > 1 {
							post = posts[1]
						} else {
							_ = threadPage.Close()
							continue
						}
					}
					aElements, err := post.Elements("a")
					if err != nil {
						fmt.Println("Get A Elements Error: ", config.BaseUrl+*href)
						panic(err)
					}

					isEmpty := true
					for _, aElement := range aElements {
						href, _ := aElement.Attribute("href")
						if strings.HasPrefix(*href, "forum.php?mod=attachment&aid=") {
							DownloadValidFile(browser, config.BaseUrl+*href, config.DownloadPath+"/"+tid, rodCookies)
							isEmpty = false
						}
					}
					if !isEmpty {
						db.Exec(`
							INSERT INTO novels (
								tid, title, content
							) VALUES (?, ?, ?)
						`, tid, titleStr, "")
					}
					threadPage.Close()
				}
			}
		}
		if menuPage.MustHas("a.nxt") {
			nextElement, err := menuPage.Element("a.nxt")
			if err != nil {
				panic(err)
			}
			nextHref, _ := nextElement.Attribute("href")
			currentMenuUrl = *nextHref
			menuPage.Close()
		} else {
			fmt.Println("Finish")
			break
		}
	}
}

func OpenValidPage(browser *rod.Browser, url string) *rod.Page {
	for {
		page := browser.MustPage()
		fmt.Println("Opening: ", url)
		err := rod.Try(func() {
			for {
				page.Timeout(10 * time.Second).MustNavigate(url)

				waitDom := page.Timeout(10 * time.Second).WaitEvent(&proto.PageDomContentEventFired{})
				waitDom()
				body := page.MustElement("body").MustText()
				if strings.TrimSpace(body) == "" {
					fmt.Println("JS Challenge")

					fmt.Println("Redirected to:", page.MustInfo().URL)
					url = page.MustInfo().URL
				} else {
					break
				}
			}
		})
		if navErr, ok := err.(*rod.NavigationError); ok {
			if navErr.Reason == "net::ERR_HTTP2_PROTOCOL_ERROR" {
				fmt.Println("Ignore HTTP/2 Protocol Error:", navErr)
				continue
			}
		} else if err != nil {
			fmt.Println("Navigate Error: ", err)
			_ = page.Close()
			ChangeProxy()
			continue
		}
		fmt.Println("Opened")

		html, err := page.HTML()
		if err != nil || len(strings.TrimSpace(html)) < 100 {
			fmt.Println("Get HTML Error: ", err)
			_ = page.Close()
			continue
		}

		pcbTexts := []string{}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err == nil {
			doc.Find("div.pcb").Each(func(i int, s *goquery.Selection) {
				pcbTexts = append(pcbTexts, s.Text())
			})
		}

		containsOutsidePCB := func(keyword string) bool {
			if !strings.Contains(html, keyword) {
				return false
			}
			for _, pcb := range pcbTexts {
				if strings.Contains(pcb, keyword) {
					return false
				}
			}
			return true
		}

		if containsOutsidePCB("您浏览的太快了，歇一会儿吧！") ||
			containsOutsidePCB("Database Error") ||
			containsOutsidePCB("502 Bad Gateway") {
			fmt.Println("Server Side Error")
			_ = page.Close()
			ChangeProxy()
			continue
		} else if containsOutsidePCB("没有找到帖子") {
			fmt.Println("Not Found Thread")
			_ = page.Close()
			return nil
		}

		return page
	}
}

func DownloadValidFile(browser *rod.Browser, url string, path string, rodCookies []*proto.NetworkCookieParam) {
	jar, _ := cookiejar.New(nil)

	u, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	cookies := []*http.Cookie{}
	for _, c := range rodCookies {
		cookies = append(cookies, &http.Cookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
		})
	}
	jar.SetCookies(u.URL, cookies)

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Jar:       jar,
	}

	for {
		resp, err := client.Get(url)
		if err != nil {
			fmt.Println("Download Error: ", err)
			ChangeProxy()
			continue
		}
		defer resp.Body.Close()

		ct := resp.Header.Get("Content-Type")

		if !strings.Contains(ct, "text/html") {
			var filename string
			cd := resp.Header.Get("Content-Disposition")
			if strings.Contains(cd, "filename=") {
				parts := strings.Split(cd, "filename=")
				if len(parts) > 1 {
					fn := strings.Trim(parts[1], "\" ")
					if fn != "" {
						filename = ConvertToUTF8(fn)
					}
				}
			}
			fmt.Println("Filename: ", filename)

			saveDir := filepath.Join(".", path)
			if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
				panic(err)
			}

			filename = CleanFileName(filename)
			fullPath := filepath.Join(saveDir, filename)
			out, err := os.Create(fullPath)
			if err != nil {
				panic(err)
			}
			defer out.Close()

			reader := &timeoutReader{r: resp.Body, timeout: 30 * time.Second}
			_, err = io.Copy(out, reader)
			if err != nil {
				fmt.Println("Read Body Error: ", err)
				ChangeProxy()
				continue
			}

			fmt.Println("Downloaded: ", fullPath)
			return
		} else {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				panic(err)
			}
			bodyStr := string(bodyBytes)

			if strings.Contains(bodyStr, "抱歉，只有特定用户可以下载本站附件") ||
				strings.Contains(bodyStr, "抱歉，该附件无法读取") ||
				strings.Contains(bodyStr, "Oops! System file lost") {
				fmt.Println("Unavailable File")
				return
			} else {
				ChangeProxy()
				continue
			}
		}
	}
}

type timeoutReader struct {
	r       io.Reader
	timeout time.Duration
}

func (tr *timeoutReader) Read(p []byte) (int, error) {
	c := make(chan struct {
		n   int
		err error
	}, 1)

	go func() {
		n, err := tr.r.Read(p)
		c <- struct {
			n   int
			err error
		}{n, err}
	}()

	select {
	case res := <-c:
		return res.n, res.err
	case <-time.After(tr.timeout):
		return 0, os.ErrDeadlineExceeded
	}
}

func ConvertToUTF8(input string) string {
	data := []byte(input)

	detector := chardet.NewTextDetector()
	result, err := detector.DetectBest(data)
	if err != nil {
		panic(err)
	}

	enc := strings.ToLower(result.Charset)
	if enc == "utf-8" {
		return input
	}

	utf8Data, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err != nil {
		panic(err)
	}

	return string(utf8Data)
}

func ChangeProxy() {
	if useProxy {
		currentProxy++
		if currentProxy%117 == 0 {
			currentProxy = 0
		}
		var useProxy string
		if currentProxy%10 == 0 {
			useProxy = "direct"
		} else {
			useProxy = strconv.Itoa(currentProxy)
		}
		url := "http://127.0.0.1:59999/proxies/selected"
		body, err := json.Marshal(map[string]string{
			"name": useProxy,
		})
		if err != nil {
			panic(err)
		}

		req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		if err != nil {
			panic(err)
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusNoContent {
			fmt.Println("Proxy Changed", useProxy)
		} else {
			fmt.Println("Change Proxy Error", string(respBody))
		}
		time.Sleep(5 * time.Second)
	} else {
		time.Sleep(5 * time.Second)
	}
}

func CleanFileName(fileName string) string {
	re := regexp.MustCompile(`[\\/:*?"<>|]`)
	clean := re.ReplaceAllString(fileName, "_")

	clean = strings.Trim(clean, " .")

	reserved := map[string]struct{}{
		"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
		"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
		"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
	}
	upper := strings.ToUpper(clean)
	if _, ok := reserved[upper]; ok {
		clean = "_" + clean
	}

	return clean
}
