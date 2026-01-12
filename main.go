package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

var (
	host string

	m map[string]struct {
		Repo    string   `yaml:"repo,omitempty"`
		Display string   `yaml:"display,omitempty"`
		Allows  []string `yaml:"allows,omitempty"`
	}
)

func init() {
	flag.StringVar(&host, "host", "", "custom domain name, e.g. tonybai.com")

	vanityBytes, err := os.ReadFile("./vanity.yaml")
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(vanityBytes, &m); err != nil {
		log.Fatal(err)
	}
	for path, entry := range m {
		if entry.Display != "" {
			continue
		}
		if strings.Contains(entry.Repo, "github.com") {
			entry.Display = fmt.Sprintf("%v %v/tree/master{/dir} %v/blob/master{/dir}/{file}#L{line}",
				entry.Repo, entry.Repo, entry.Repo)
			m[path] = entry // 写回修改后的结构体
		}
	}
}

// 获取客户端真实IP（支持常见代理头）
func realIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		parts := strings.SplitN(ip, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}

	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// 判断 IP 是否在允许列表中（支持 CIDR 和单 IP）
func ipAllowed(clientIP string, allows []string) bool {
	if len(allows) == 0 {
		return true // 未配置 = 全部放行
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, rule := range allows {
		rule = strings.TrimSpace(rule)
		if rule == clientIP {
			return true
		}

		_, ipnet, err := net.ParseCIDR(rule)
		if err == nil && ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

func handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 获取该路径的配置
	conf, ok := m[path]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// IP 白名单检查
	clientIP := realIP(r)
	if !ipAllowed(clientIP, conf.Allows) {
		log.Printf("Forbidden: %s → %s", clientIP, path)
		//http.Error(w, "403 Forbidden - IP not allowed", http.StatusForbidden)
		http.Redirect(w, r, "https://yuanc.com", http.StatusFound)
		return
	}

	// 渲染模板
	if err := vanityTmpl.Execute(w, struct {
		Import  string
		Repo    string
		Display string
	}{
		Import:  strings.TrimRight(host, "/") + "/" + strings.TrimLeft(path, "/"),
		Repo:    conf.Repo,
		Display: conf.Display,
	}); err != nil {
		http.Error(w, "cannot render template", http.StatusInternalServerError)
	}
}

var vanityTmpl, _ = template.New("vanity").Parse(`<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
<meta name="go-import" content="{{.Import}} git {{.Repo}}">
<meta name="go-source" content="{{.Import}} {{.Display}}">
<meta http-equiv="refresh" content="0; url=https://godoc.org/{{.Import}}">
</head>
<body>
Nothing to see here; <a href="https://godoc.org/{{.Import}}">see the package on godoc</a>.
</body>
</html>`)

func usage() {
	fmt.Println("govanityurls - custom go import path service")
	fmt.Println("Usage:")
	fmt.Println("\t govanityurls -host example.com")
	flag.PrintDefaults()
}

func main() {
	flag.Parse()

	if host == "" {
		usage()
		return
	}

	if !strings.HasSuffix(host, "/") {
		host += "/"
	}

	log.Printf("Starting govanityurls | host: %s | listening on :8080", host)
	log.Fatal(http.ListenAndServe(":8080", http.HandlerFunc(handle)))
}
