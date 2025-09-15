package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
					html, err := threadPage.HTML()
					if err != nil {
						panic(err)
					}

					if strings.Contains(html, "没有找到帖子") || strings.Contains(html, "Database Error") {
						fmt.Println("Not Found")
						_ = threadPage.Close()
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
							threadPage.Close()
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
		err := rod.Try(func() {
			wait := page.WaitEvent(proto.PageDomContentEventFired{})
			page.Timeout(10 * time.Second).MustNavigate(url)
			wait()
		})
		if err != nil {
			fmt.Println("Navigate Error: ", err)
			_ = page.Close()
			ChangeProxy()
			continue
		}

		html, err := page.HTML()
		if err != nil {
			fmt.Println("Get HTML Error: ", err)
			_ = page.Close()
			continue
		}

		if strings.Contains(html, "您浏览的太快了，歇一会儿吧！") ||
			strings.Contains(html, "Database Error") ||
			strings.Contains(html, "502 Bad Gateway") {
			fmt.Println("Server Side Error")
			_ = page.Close()
			ChangeProxy()
			continue
		} else if strings.Contains(html, "没有找到帖子") {
			fmt.Println("Not Found Thread")
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

	client := &http.Client{Jar: jar}

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

			fullPath := filepath.Join(saveDir, filename)
			out, err := os.Create(fullPath)
			if err != nil {
				panic(err)
			}
			defer out.Close()

			_, err = io.Copy(out, resp.Body)
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

			if strings.Contains(bodyStr, "抱歉，只有特定用户可以下载本站附件") || strings.Contains(bodyStr, "抱歉，该附件无法读取") {
				fmt.Println("Only Unique")
				return
			} else {
				ChangeProxy()
				continue
			}
		}
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
		time.Sleep(time.Second)
	} else {
		time.Sleep(5 * time.Second)
	}
}
