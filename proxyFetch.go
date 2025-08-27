package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html/charset"
)

// Task 包含一个返回结果的通道
type Task struct {
	Url    string
	Result chan Result
}

// Result 用来存放请求结果
type Result struct {
	Body []byte
	Err  error
}

var (
	taskCh = make(chan Task, 100)
	once   sync.Once
)

const getProxyStr = "http://api2.xkdaili.com/tools/XApi.ashx?apikey=XK18C8EF13FE102BF294&qty=1&format=txt&split=0&sign=9bcab6037ea8275aebe49d5c04989c4a"

func getProxy() (string, error) {
	for i := 0; i < 5; i++ { // 最多尝试5次
		resp, err := http.Get(getProxyStr)
		if err != nil {
			return "", err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", err
		}

		proxy := "http://" + strings.TrimSpace(string(body))
		if checkProxy(proxy) {
			fmt.Println("代理可用")
			return proxy, nil
		}
		fmt.Println("代理不可用，尝试获取新代理...")
		time.Sleep(time.Second) // 等待1秒再尝试
	}
	return "", fmt.Errorf("无法获取可用代理")
}

// checkProxy 测试代理是否可用
func checkProxy(proxy string) bool {
	proxyURL, _ := url.Parse(proxy)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 5 * time.Second, // 超时设置
	}

	resp, err := client.Get("https://4.ipw.cn")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	content := string(body)
	// 简单判断返回内容是否包含 IP 格式
	if strings.Contains(content, ".") {
		return true
	}
	return false
}

// 用代理处理一个任务
func processTask(task Task, proxy string) {
	proxyURL, _ := url.Parse(proxy)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest("GET", task.Url, nil)
	if err != nil {
		task.Result <- Result{Body: []byte{}, Err: fmt.Errorf("error creating request: %w", err)}
	}

	req.Header.Set("Cookie", "yj0M_eda4_saltkey=Zj2pqQCn; yj0M_eda4_lastvisit=1756258948; yj0M_eda4_lastact=1756262702%09index.php%09; PHPSESSID=ollfild76oisbps0h8shvvnh7l; yj0M_eda4_st_t=2527028%7C1756262620%7C847996d4cad16b3af85a3f76d1da95b6; yj0M_eda4_sendmail=1; yj0M_eda4_ulastactivity=1756262607%7C0; yj0M_eda4_auth=cb86O5Ui0pLJxSEbgg7pzR9RVTGzjUotZC5Bd5e2KlpjlUFN9zqA7ySaR5AApNnLc6aMKhfgwqqd9uXBQDNsDANcbP5C; yj0M_eda4_lastcheckfeed=2527028%7C1756262607; yj0M_eda4_lip=221.6.242.203%2C1756262607; yj0M_eda4_sid=0; yj0M_eda4_forum_lastvisit=D_102_1756262620")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0")

	resp, err := client.Do(req)
	if err != nil {
		task.Result <- Result{Body: []byte{}, Err: err}
		return
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	reader, err := charset.NewReader(resp.Body, contentType)
	if err != nil {
		task.Result <- Result{Body: []byte{}, Err: fmt.Errorf("charset reader: %w", err)}
	}

	buf, err := io.ReadAll(reader)
	if err != nil {
		task.Result <- Result{Body: []byte{}, Err: fmt.Errorf("read body: %w", err)}
	}

	task.Result <- Result{Body: buf, Err: nil}
}

// 调度器：一个代理用 10 次后换新代理
func scheduler() {
	var currentProxy string
	var count int

	for task := range taskCh {
		// 没有代理或代理已用 10 次，就换一个
		if currentProxy == "" || count >= 10 {
			proxy, err := getProxy()
			if err != nil {
				task.Result <- Result{Body: []byte{}, Err: fmt.Errorf("获取代理失败: %w", err)}
				continue
			}
			currentProxy = proxy
			count = 0
			fmt.Println("切换代理:", currentProxy)
		}

		go processTask(task, currentProxy)
		count++
	}
}

// 外部调用函数，返回一个结果通道
func Fetch(url string) <-chan Result {
	once.Do(func() {
		go scheduler()
	})

	resultCh := make(chan Result, 1)
	taskCh <- Task{Url: url, Result: resultCh}
	return resultCh
}
