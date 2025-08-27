package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type Board struct {
	UrlString   string
	TotalPage   int
	ArchiveTime int
}

const domainName = "3cbg9.sdgvre54q.com"
const baseUrlString = "https://3cbg9.sdgvre54q.com/"

var boards = [6]Board{
	{UrlString: "forum.php?mod=forumdisplay&fid=102", TotalPage: 355, ArchiveTime: 2015},
	{UrlString: "forum.php?mod=forumdisplay&fid=101", TotalPage: 338, ArchiveTime: 2018},
	{UrlString: "forum.php?mod=forumdisplay&fid=100", TotalPage: 730, ArchiveTime: 2020},
	{UrlString: "forum.php?mod=forumdisplay&fid=99", TotalPage: 381, ArchiveTime: 2021},
	{UrlString: "forum.php?mod=forumdisplay&fid=104", TotalPage: 1889, ArchiveTime: 2022},
	{UrlString: "forum.php?mod=forumdisplay&fid=103", TotalPage: 923, ArchiveTime: 2023},
}

func main() {
	mobileTXTUrlString := boards[0].UrlString
	totalPages := boards[0].TotalPage
	totalPages = 12
	var wg sync.WaitGroup

	tidChannel := make(chan int)
	for i := 1; i <= totalPages; i++ {
		wg.Add(1)
		BookListPageUrlString := baseUrlString + mobileTXTUrlString + "&page=" + strconv.Itoa(i)
		go scrapeBookListPage(BookListPageUrlString, tidChannel, &wg)
	}

	go func() {
		wg.Wait()
		close(tidChannel)
	}()

	var tids []int
	for v := range tidChannel {
		tids = append(tids, v)
	}
	_ = os.WriteFile("data.json", must(json.Marshal(tids)), 0644)
	fmt.Printf("✡️ Finished! total:%d\n", len(tids))
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func scrapeBookListPage(page string, ch chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("🌟 爬取页面: %s\n", page)
	respCh := Fetch(page)
	result := <-respCh // 等待结果
	if result.Err != nil {
		fmt.Println("请求失败:", result.Err)
		return
	}

	reader := bytes.NewReader(result.Body)
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		panic(err)
	}

	doc.Find("#threadlisttableid > tbody").Each(func(i int, s *goquery.Selection) {
		id, exists := s.Attr("id")
		if exists {
			if strings.Contains(id, "normalthread") {
				href, exists := s.Find("tr:nth-child(1) > th:nth-child(2) > a:nth-child(3)").Attr("href")
				if exists {
					u, err := url.Parse(href)
					if err != nil {
						panic(err)
					}

					query, err := url.ParseQuery(u.RawQuery)
					if err != nil {
						panic(err)
					}

					tid := query.Get("tid")

					if tid != "adver" {
						tidInt, err := strconv.Atoi(tid)
						if err != nil {
							panic(err)
						}
						ch <- tidInt
					}
				}
			}
		}
	})
}
