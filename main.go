package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
	// "github.com/PuerkitoBio/goquery"
	// _ "modernc.org/sqlite"
)

var db *sql.DB

const baseUrlString = "https://ac1ss.ascwefkjw.com/"
const mobileTXTUrlString = "forum.php?mod=forumdisplay&fid=40"

func main() {
	// db, _ = sql.Open("sqlite", "soushu-mobileTXT.db")
	// defer db.Close()

	// createTableSQL := `
	// CREATE TABLE IF NOT EXISTS novel (
	//     tid INTEGER PRIMARY KEY,
	//     title TEXT,
	//     content TEXT
	// );`
	// _, err := db.Exec(createTableSQL)
	// if err != nil {
	// 	panic(err)
	// }
	scrapeBookListPage(1)
}

func fetchWithProxy(targetURL string, proxyStr string, timeout time.Duration) (string, error) {
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return "", fmt.Errorf("invalid proxy URL %s: %w", proxyStr, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Cookie", "PHPSESSID=fehap6pvv43kuu7r4foskqi2dp; yj0M_5a0c_ulastactivity=1755685095%7C0; yj0M_5a0c_saltkey=wTb5t5gB; yj0M_5a0c_lastvisit=1755680847; yj0M_5a0c_lastact=1755685522%09forum.php%09viewthread; yj0M_5a0c__refer=%252Fhome.php%253Fmod%253Dspacecp%2526ac%253Dprofile%2526op%253Dpassword; yj0M_5a0c_auth=cf10GXGgtSukhwCS4cImZ9JYBSVvTghCdTsq1ht4qyebLeDNdTprQaXQm7vXC%2FDyke9L51ISDV24Z%2FENcwBPiu43hek; yj0M_5a0c_lastcheckfeed=588604%7C1755684455; yj0M_5a0c_lip=47.79.94.249%2C1755684455; yj0M_5a0c_sid=0; yj0M_5a0c_nofavfid=1; yj0M_5a0c_st_t=588604%7C1755685250%7C1cb744b1014d3a5af96469584e0de3cb; yj0M_5a0c_forum_lastvisit=D_102_1755684843D_72_1755684870D_103_1755685011D_40_1755685250; yj0M_5a0c_smile=1D1; yj0M_5a0c_st_p=588604%7C1755685522%7C591978a0f584979afc565ead1ec9e755; yj0M_5a0c_viewid=tid_1324402")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Go-http-client/1.1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error accessing %s via proxy %s: %w", targetURL, proxyStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 status code %d from %s", resp.StatusCode, targetURL)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response from %s: %w", targetURL, err)
	}

	return string(body), nil
}

// func scrapeBookPage(bookUrlString string) {
// 	bookUrl, _ := url.Parse(baseUrlString)

// 	parsed, _ := url.Parse(bookUrlString)

// 	temp := parsed.Query()
// 	temp.Del("extra")
// 	parsed.RawQuery = temp.Encode()

// 	bookUrl.Path = parsed.Path
// 	bookUrl.RawQuery = parsed.RawQuery

// 	bookReq := req
// 	bookReq.URL = bookUrl
// 	fmt.Println("bookUrl")
// 	fmt.Println(bookUrl.String())

// 	client := &http.Client{}
// 	resp, err := client.Do(bookReq)
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer resp.Body.Close()

// 	var reader io.ReadCloser
// 	if resp.Header.Get("Content-Encoding") == "gzip" {

// 		reader, err = gzip.NewReader(resp.Body)
// 		if err != nil {
// 			panic(err)
// 		}
// 		defer reader.Close()
// 	} else {
// 		reader = resp.Body
// 	}

// 	body, err := io.ReadAll(reader)
// 	if err != nil {
// 		panic(err)
// 	}
// 	fmt.Println(string(body))

// 	doc, err := goquery.NewDocumentFromReader(reader)
// 	if err != nil {
// 		panic(err)
// 	}

// 	tid := bookUrl.Query().Get("tid")

// 	title := doc.Find("#thread_subject").Text()

// 	var content string

// 	downloadElement := doc.Find(".attnm a").First()

// 	href, exists := downloadElement.Attr("href")
// 	if exists {

// 		downloadUrl, _ := url.Parse(baseUrlString)

// 		parsed, _ := url.Parse("/" + href)
// 		downloadUrl.Path = parsed.Path
// 		downloadUrl.RawQuery = parsed.RawQuery

// 		bookReq.URL = downloadUrl
// 		fmt.Println("downloadUrl")
// 		fmt.Println(downloadUrl.String())
// 		downloadClient := &http.Client{}
// 		downloadResp, err := downloadClient.Do(bookReq)
// 		if err != nil {
// 			panic(err)
// 		}
// 		defer downloadResp.Body.Close()

// 		var downloadReader io.ReadCloser
// 		downloadReader = downloadResp.Body
// 		body, err := io.ReadAll(downloadReader)
// 		content = string(body)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}

// 	insertSQL := `INSERT INTO novel (tid, title, content) VALUES (?, ?, ?)`
// 	_, err = db.Exec(insertSQL, tid, title, content)
// 	if err != nil {
// 		panic(err)
// 	}
// }

func scrapeBookListPage(page int) {
	BookListPageUrlString := baseUrlString + mobileTXTUrlString + "&page=" + strconv.Itoa(page)
	fmt.Println(BookListPageUrlString)
	proxyStr := fmt.Sprintf("http://oc-ab35117cd53146a5be0297f8b368339cb877481a6178015c64d8599f75127596-country-US-session-%d:it5q47pegzmt@proxy.oculus-proxy.com:31114", page)
	body, err := fetchWithProxy(BookListPageUrlString, proxyStr, 600*time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Println(body)
}
