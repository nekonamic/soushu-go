package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	_ "modernc.org/sqlite"

	"soushu-go/cookies"
)

type Config struct {
	BaseUrl      string `json:"baseUrl"`
	PathUrl      string `json:"pathUrl"`
	Type         int8   `json:"type"`
	DownloadPath string `json:"downloadPath"`
}

func main() {
	configRaw, err := os.ReadFile("./config.json")
	if err != nil {
		panic(err)
	}

	var config Config
	if err := json.Unmarshal(configRaw, &config); err != nil {
		panic(err)
	}

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

	browser := rod.New().MustConnect()
	defer browser.MustClose()
	browser.SetCookies(cookies.Get())

	fmt.Println(config.BaseUrl + config.PathUrl)
	menuPage := OpenValidPage(browser, config.BaseUrl+config.PathUrl)

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
				fmt.Println("adver")
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
				fmt.Println("exists")
			} else {
				threadPage := OpenValidPage(browser, config.BaseUrl+*href)
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
				fmt.Println(titleStr)

				posts, err := threadPage.Elements("div.pcb")
				if err != nil {
					fmt.Println("Get Post Error: ", config.BaseUrl+*href)
					panic(err)
				}
				for _, post := range posts {
					aElements, err := post.Elements("a")
					if err != nil {
						fmt.Println("Get A Elements Error: ", config.BaseUrl+*href)
						panic(err)
					}
					for _, aElement := range aElements {
						href, _ := aElement.Attribute("href")
						if strings.HasPrefix(*href, "forum.php?mod=attachment&aid=") {
							fmt.Println(*href)
						}
					}
				}
			}
		}
	}
}

func OpenValidPage(browser *rod.Browser, url string) *rod.Page {
	for {
		page, err := browser.Page(proto.TargetCreateTarget{URL: url})
		if err != nil {
			fmt.Println("Create Page Error")
			panic(err)
		}

		if err := page.WaitLoad(); err != nil {
			fmt.Println("Wait Load Error: ", err)
			_ = page.Close()
			continue
		}

		html, err := page.HTML()
		if err != nil {
			fmt.Println("Get HTML Error: ", err)
			_ = page.Close()
			continue
		}

		if strings.Contains(html, "您浏览的太快了，歇一会儿吧！") || strings.Contains(html, "Database Error") {
			fmt.Println("Too Fast or Database Error")
			_ = page.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		return page
	}
}

// func DownloadValidFile(browser *rod.Browser, url string) {
// 	for {
// 		page, err := browser.Page(proto.TargetCreateTarget{URL: url})
// 		if err != nil {
// 			fmt.Println("Create Page Error")
// 			panic(err)
// 		}

// 		contentType := ""
// 		done := make(chan struct{})

// 		cancel := page.Browser().EachEvent(func(e *proto.NetworkResponseReceived) {
// 			if e.Type == proto.NetworkResourceTypeDocument && e.Response.URL == url {
// 				if ct, ok := e.Response.Headers["content-type"]; ok {
// 					contentType = strings.ToLower(ct.String())
// 				}
// 				close(done)
// 			}
// 		})
// 		defer cancel()

// 		<-done

// 		if strings.HasPrefix(contentType, "text/html") {
// 			fmt.Println("Not File")
// 			continue
// 		}

// 		resp, err := http.Get(url)
// 		if err != nil {
// 			return nil, "", err
// 		}
// 		defer resp.Body.Close()

// 		// 确定文件名
// 		filename := "downloaded_file"
// 		if strings.Contains(url, "/") {
// 			parts := strings.Split(url, "/")
// 			filename = parts[len(parts)-1]
// 		}
// 		filepath := downloadDir + "/" + filename

// 		// 保存文件
// 		out, err := os.Create(filepath)
// 		if err != nil {
// 			return nil, "", err
// 		}
// 		defer out.Close()
// 		_, err = io.Copy(out, resp.Body)
// 		if err != nil {
// 			return nil, "", err
// 		}

// 		// 下载完成，关闭 page
// 		_ = page.Close()
// 		return nil, filepath, nil
// 	}
// }
