package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/julienschmidt/httprouter"
	"github.com/urfave/negroni"
	"github.com/xormplus/xorm"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/alecthomas/kingpin.v2"

	_ "github.com/go-sql-driver/mysql"
)

var (
	task          = make(chan int, 10)
	ch            = make(chan Site, 1)
	logInfo       *log.Logger
	logLoop       *log.Logger
	logWarn       *log.Logger
	logError      *log.Logger
	db            *xorm.Engine
	maxRoutineNum = kingpin.Flag("maxRoutineNum", "refresh status routine num").Default("10").Int()
	port          = kingpin.Flag("port", "listen http port").Short('p').String()
	scLogDir      = kingpin.Flag("log-dir", "log file path").Default("log").ExistingDir()
	scLogFileName = kingpin.Flag("log-file-name", "log file name").Default("xping.log").String()
	scinstall     = kingpin.Flag("install", "install program").Bool()
)

//Site struct
//V6 V4 开头的属性，默认值为1 2代表支持
type Site struct {
	ID      int       `json:"id" xorm:"pk autoincr 'id'"`
	Domain  string    `json:"domain" xorm:"domain"`
	Desc    string    `json:"desc" xorm:"desc"`
	IPv6    string    `json:"ipv6" xorm:"ipv6"`
	IPv4    string    `json:"ipv4" xorm:"ipv4"`
	V6hp    int       `json:"v6hp" xorm:"v6hp"` //检测是否有v6 http
	V4hp    int       `json:"v4hp" xorm:"v4hp"` //检测是否有v4 http
	V6hs    int       `json:"v6hs" xorm:"v6hs"` //检测是否有v6 https
	V4hs    int       `json:"v4hs" xorm:"v4hs"` //检测是否有v4 https
	V6h2    int       `json:"v6h2" xorm:"v6h2"` //检测是否有v6 htt2
	V4h2    int       `json:"v4h2" xorm:"v4h2"` //检测是否有v4 htt2
	CETime  time.Time `json:"cetime" xorm:"cetime"`
	V6time  time.Time `json:"v6time" xorm:"v6time"`
	Created time.Time `json:"created" xorm:"created"`
	Updated time.Time `json:"updated" xorm:"updated"`
}

//Lable struct
//want now
type Lable struct {
	ID       int       `json:"id"  xorm:"pk autoincr 'id'" `
	SID      int       `json:"sid"  xorm:"sid"`
	Lable    string    `json:"lable" xorm:"lable"`
	Classify string    `json:"classify" xorm:"classify"`
	Created  time.Time `json:"created" xorm:"created"`
	Updated  time.Time `json:"updated" xorm:"updated"`
}

// Er struct
type Er struct {
	Ret   string      `json:"ret"`
	Msg   string      `json:"msg"`
	Data  interface{} `json:"data"`
	Param string      `json:"param"`
}

type school struct {
	Domain   string `json:"domain"`
	Tag      string `json:"tag"`
	Name     string `json:"name"`
	Classify string `json:"classify"`
}

func init() {
	kingpin.Parse()
	var e error
	params := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8&parseTime=true", "root", "qwerty", "mysql:3306", "v6sc")
	db, e = xorm.NewMySQL("mysql", params)
	if e != nil {
		panic(e)
	}
	task = make(chan int, *maxRoutineNum)
	go func() {
		for {
			select {
			case s := <-ch:
				db.ID(s.ID).Update(&s)
			}
		}
	}()
}

func install() error {
	if e := db.Ping(); e != nil {
		return e
	}
	return db.CreateTables(&Site{}, &Lable{})
}

func main() {
	logFile, e := os.OpenFile(fmt.Sprintf("%s/%s", *scLogDir, *scLogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if e != nil {
		log.Fatalln("打开日志文件失败：", e)
	}

	if *scinstall {
		install()
		os.Exit(0)
	}

	logInfo = log.New(io.MultiWriter(os.Stdout, logFile), "[info] ", log.Ldate|log.Ltime|log.Lshortfile)
	logLoop = log.New(io.MultiWriter(os.Stdout, logFile), "[loop] ", log.Ldate|log.Ltime|log.Lshortfile)
	logWarn = log.New(io.MultiWriter(os.Stdout, logFile), "[warn] ", log.Ldate|log.Ltime|log.Lshortfile)
	logError = log.New(io.MultiWriter(os.Stdout, logFile), "[error] ", log.Ldate|log.Ltime|log.Lshortfile)

	httpLog := negroni.NewLogger()
	httpLog.ALogger = log.New(io.MultiWriter(os.Stdout, logFile), "[https] ", 0)
	httpLog.SetFormat("{{.StartTime}} {{.Hostname}} {{.Duration}} [{{.Method}} {{.Request.Proto}} {{.Status}} {{.Path}}] {{.Request.RemoteAddr}} {{.Request.UserAgent}}")

	mux := httprouter.New()
	// mux.PanicHandler = func(w http.ResponseWriter, r *http.Request, v interface{}) {
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	logError.Println(v)
	// }
	mux.GET("/", indexHTML)
	mux.GET("/renewal", renewal)
	mux.GET("/index", indexHTML)
	mux.GET("/testsite", testsite)
	mux.GET("/justSupport", justSupport)
	mux.GET("/searchsite", searchsite)
	mux.GET("/addsite", addsite)
	mux.GET("/cityuniversitydetail", cityuniversitydetail)
	mux.ServeFiles("/static/*filepath", http.Dir("./static"))
	if *port != "" {
		fmt.Printf("http://127.0.0.1:%s\n", *port)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", *port), mux))
	}
	n := negroni.New()
	n.UseHandler(mux)
	n.Use(httpLog)
	m := autocert.Manager{
		Cache:      autocert.DirCache(".letsencrypt"),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("v6sc.ipip.net."),
		Email:      "zhangyuan@newyou.ltd",
	}
	go http.ListenAndServe(":80", m.HTTPHandler(nil))
	ss := &http.Server{
		Addr:           ":443",
		MaxHeaderBytes: 1 << 20,
		Handler:        n,
		TLSConfig:      &tls.Config{GetCertificate: m.GetCertificate},
	}
	ss.ListenAndServeTLS("", "")
}

func justSupport(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var n = req.URL.Query().Get("n")
	b, _ := strconv.Atoi(n)
	if b < 0 || b > 5 {
		return
	}
	log.Printf("Method %s RemoteAddr %s User-Agent %s URL %s Behavior Load More Just Support%s\n", req.Method, req.RemoteAddr, req.UserAgent(), req.URL.String(), n)
	var latestSupportV6 []Site
	if err := db.Where("v6time is not null").Desc("v6time").Limit(20, (b-1)*20).Find(&latestSupportV6); err != nil {
		panic(err)
	}

	var dom = `{{range $k,$v := .latestSupportV6}}
	<tr>
		<td class="align-middle">{{$v.Domain}}</td>
		<td class="align-middle">{{$v.Desc}}</td>
		<td class="align-middle">{{$v.IPv4}}</td>
		<td>{{if eq $v.V4hp 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td>{{checkCertificate $v 4}}</td>
		<td>{{if eq $v.V4h2 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td class="align-middle">{{$v.IPv6}}</td>
		<td>{{if eq $v.V6hp 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td>{{checkCertificate $v 6}}</td>
		<td>{{if eq $v.V6h2 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td class="align-middle">{{$v.Created.Format "2006-01-02 15:04"}}</td>
		<td class="align-middle">{{$v.Updated.Format "2006-01-02 15:04"}}</td>
		<td class="align-middle"><a href="javascript:renewal({{$v.ID}})">更新</a></td>
	</tr>
	{{end}}`
	t, _ := template.New("dom").Funcs(template.FuncMap{"checkCertificate": checkCertificate}).Parse(dom)
	t.Execute(w, map[string]interface{}{"latestSupportV6": latestSupportV6})
}

func searchsite(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var domain = req.URL.Query().Get("domain")
	if domain == "" {
		return
	}
	var res []Site
	err := db.Where("domain like ?", "%"+domain+"%").Desc("id").Limit(20, 0).Find(&res)
	if err != nil {
		return
	}
	var dom = `{{range $k,$v := .res}}
	<tr>
		<td class="align-middle">{{$v.Domain}}</td>
		<td class="align-middle">{{$v.Desc}}</td>
		<td class="align-middle">{{$v.IPv4}}</td>
		<td> {{if eq $v.V4hp 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td>{{checkCertificate $v 4}}</td>
		<td> {{if eq $v.V4h2 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td class="align-middle">{{viewIPv6 $v.IPv6}}</td>
		<td> {{if eq $v.V6hp 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td>{{checkCertificate $v 6}}</td>
		<td> {{if eq $v.V6h2 2}}<button type="button" class="btn btn-outline-success btn-sm">已支持</button>{{else}}<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>{{end}}</td>
		<td class="align-middle">{{$v.Created.Format "2006-01-02 15:04"}}</td>
		<td class="align-middle">{{$v.Updated.Format "2006-01-02 15:04"}}</td>
		<td class="align-middle"><a href="javascript:renewal({{$v.ID}})">更新</a></td>
	</tr>
	{{end}}`
	t, _ := template.New("dom").Funcs(template.FuncMap{
		"checkCertificate": checkCertificate,
		"viewIPv6":         viewIPv6,
	}).Parse(dom)
	t.Execute(w, map[string]interface{}{"res": res})
}

func testsite(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var domain = req.URL.Query().Get("domain")
	wSite := &Site{Domain: domain}
	res, ge := db.Get(wSite)
	if ge != nil {
		panic(ge)
	}
	if res {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "此域名已有记录，你可以再搜索中找到它"})
		w.Write(msg)
		return
	}
	if ip := net.ParseIP(domain); ip != nil {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "不能是IP"})
		w.Write(msg)
		return
	}
	ns, err := net.LookupHost(domain)
	if err != nil || len(ns) < 1 {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "没有dns记录"})
		w.Write(msg)
		return
	}
	w.WriteHeader(http.StatusOK)
	msg, _ := json.Marshal(Er{Ret: "v"})
	w.Write(msg)
}

func addsite(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var domain = req.URL.Query().Get("domain")
	var desc = req.URL.Query().Get("desc")
	wSite := &Site{Domain: domain}
	//先查库里有没有
	res, ge := db.Get(wSite)
	if ge != nil {
		panic(ge)
	}
	if res {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "此域名已有记录，你可以再搜索中找到它"})
		w.Write(msg)
		return
	}
	descLen := utf8.RuneCountInString(desc)
	if descLen < 2 {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "描述过短"})
		w.Write(msg)
		return
	}
	if descLen > 10 {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "描述过长"})
		w.Write(msg)
		return
	}
	if ip := net.ParseIP(domain); ip != nil {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "不能是IP"})
		w.Write(msg)
		return
	}
	ns, err := net.LookupHost(domain)
	if err != nil || len(ns) < 1 {
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "e", Msg: "没有dns记录"})
		w.Write(msg)
		return
	}
	var site Site
	site.Domain = domain
	for _, s := range ns {
		if net.ParseIP(s).To4() != nil {
			site.IPv4 = s
		} else {
			site.IPv6 = s
		}
	}
	site.Desc = desc
	_, err = db.Insert(&site)
	if err != nil {
		panic(err)
	}

	go func() {
		task <- 1
		checkDomain(site)
	}()
	w.WriteHeader(http.StatusOK)
	msg, _ := json.Marshal(Er{Ret: "v", Msg: "添加完毕"})
	w.Write(msg)
}

func indexHTML(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var latestDomain []Site
	var latestSupportV6 []Site
	var willExpire []Site

	if err := db.Where("v6time is not null").Desc("v6time").Limit(20, 0).Find(&latestSupportV6); err != nil {
		panic(err)
	}
	if err := db.Where("cetime is not null and cetime < ?", time.Now().UTC().Add(time.Hour*time.Duration(30*24))).Asc("cetime").Find(&willExpire); err != nil {
		panic(err)
	}

	if err := db.Desc("created").Limit(20, 0).Find(&latestDomain); err != nil {
		panic(err)
	}

	type universityDetail struct {
		Site  `xorm:"extends"`
		Lable `xorm:"extends"`
	}

	var universityDetails []universityDetail
	if err := db.Table("lable").Join("INNER", "site", "site.id=lable.sid").Where("lable.classify=?", "university").Find(&universityDetails); err != nil {
		panic(err)
	}

	var universityClassify = make(map[string][]Site)

	var (
		supportIpv6Count      = 0
		supportIpv6HttpCount  = 0
		supportIpv6HttpsCount = 0
		supportIpv6Http2Count = 0
	)

	for _, university := range universityDetails {
		if university.IPv6 != "" {
			supportIpv6Count++
		}
		if university.V6hs == 2 {
			supportIpv6HttpsCount++
		}
		if university.V6hp == 2 {
			supportIpv6HttpCount++
		}
		if university.V6h2 == 2 {
			supportIpv6Http2Count++
		}
		universityClassify[university.Lable.Lable] = append(universityClassify[university.Lable.Lable], university.Site)
	}

	var universityStat = make(map[string]int)

	universityStat["supportIpv6Count"] = supportIpv6Count
	universityStat["supportIpv6HttpCount"] = supportIpv6HttpsCount
	universityStat["supportIpv6HttpsCount"] = supportIpv6HttpsCount
	universityStat["supportIpv6Http2Count"] = supportIpv6Http2Count
	if len(universityDetails) > 0 {
		universityStat["count"] = len(universityDetails)
		universityStat["supportIpv6Scale"] = supportIpv6Count * 100 / len(universityDetails)
		universityStat["supportIpv6HttpScale"] = supportIpv6HttpCount * 100 / len(universityDetails)
		universityStat["supportIpv6HttpsScale"] = supportIpv6HttpsCount * 100 / len(universityDetails)
		universityStat["supportIpv6Http2Scale"] = supportIpv6Http2Count * 100 / len(universityDetails)
	} else {
		universityStat["count"] = 0
		universityStat["supportIpv6Scale"] = 0
		universityStat["supportIpv6HttpScale"] = 0
		universityStat["supportIpv6HttpsScale"] = 0
		universityStat["supportIpv6Http2Scale"] = 0
	}

	var (
		siteCount      int64
		supportV6Count int64
	)

	siteCount, e := db.Table("site").Count()
	if e != nil {
		panic(e)
	}
	supportV6Count, e = db.Table("site").Where("IPv6 != ''").Count()
	if e != nil {
		panic(e)
	}

	var siteStat = make(map[string]int)
	siteStat["count"] = int(siteCount)
	siteStat["supportV6Count"] = int(supportV6Count)
	if siteCount != 0 {
		siteStat["supportV6Scale"] = int(supportV6Count * 100 / siteCount)
	} else {
		siteStat["supportV6Scale"] = 0
	}
	t, _ := template.New("index.html").Funcs(template.FuncMap{"checkCertificate": checkCertificate, "viewIPv6": viewIPv6, "universityCount": universityCount}).ParseFiles("views/index.html")

	t.Execute(w, map[string]interface{}{
		"siteStat":           siteStat,
		"universityStat":     universityStat,
		"latestDomain":       latestDomain,
		"willExpire":         willExpire,
		"latestSupportV6":    latestSupportV6,
		"universityDetails":  universityDetails,
		"universityClassify": universityClassify,
	})
}

func renewal(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var id, _ = strconv.Atoi(req.URL.Query().Get("id"))
	site := Site{ID: id}
	res, ge := db.Get(&site)
	if ge != nil {
		panic(ge)
	}
	if res {
		go func() {
			task <- 1
			checkDomain(site)
		}()
		w.WriteHeader(http.StatusOK)
		msg, _ := json.Marshal(Er{Ret: "v", Msg: "已经加入列队"})
		w.Write(msg)
		return
	}
	w.WriteHeader(http.StatusOK)
	msg, _ := json.Marshal(Er{Ret: "e", Msg: "没有这个记录"})
	w.Write(msg)
	return
}

func viewIPv6(ip string) string {
	var newIP = ip
	if len(ip) > 20 {
		newIP = fmt.Sprintf("%s...", ip[:strings.Index(":", ip[:18])+18])
	}
	return fmt.Sprintf("<span data-toggle='tooltip' data-placement='top' data-html='true' title='%s'>%s</span>", ip, newIP)
}
func checkCertificate(site Site, v int) string {
	var p int
	if v == 4 {
		p = site.V4hs
	} else {
		p = site.V6hs
	}
	if p != 2 {
		return `<button type="button" class="btn btn-outline-danger btn-sm">不支持</button>`
	}
	if site.CETime.IsZero() {
		return `<button type="button" class="btn btn-outline-success btn-sm">已支持</button>`
	}
	if site.CETime.Before(time.Now()) {
		var title = "证书今天刚过期"
		if int(time.Now().Sub(site.CETime).Hours())/24 > 0 {
			title = fmt.Sprintf("证书已在%d天前过期", int(time.Now().Sub(site.CETime).Hours())/24)
		}
		return fmt.Sprintf(`<button type="button" class="btn btn-danger btn-sm" data-toggle="tooltip" data-placement="top" title="%s">已过期</button>`, title)
	}
	if site.CETime.Before(time.Now().AddDate(0, 1, 0)) {
		return fmt.Sprintf(`<button type="button" class="btn btn-outline-warning btn-sm" data-toggle="tooltip" data-placement="top" title="%d天后证书过期">已支持</button>`, int(site.CETime.Sub(time.Now()).Hours())/24)
	}
	return `<button type="button" class="btn btn-outline-success btn-sm">已支持</button>`
}

func universityCount(x string, y []Site) string {
	var university = make(map[string]interface{})
	var len = len(y)
	var (
		ipv6P int
		v6hsP int
		v6h2P int
		v6hpP int
	)
	for _, un := range y {
		if un.IPv6 != "" {
			ipv6P++
		}
		if un.V6hs == 2 {
			v6hsP++
		}
		if un.V6h2 == 2 {
			v6h2P++
		}
		if un.V6hp == 2 {
			v6hpP++
		}
	}

	university["city"] = x
	university["count"] = len
	university["ipv6Count"] = ipv6P
	university["ipv6Scale"] = ipv6P * 100 / len
	university["v6hpCount"] = v6hpP
	university["v6hpScale"] = v6hpP * 100 / len
	university["v6hsCount"] = v6hsP
	university["v6hsScale"] = v6hsP * 100 / len
	university["v6h2Count"] = v6h2P
	university["v6h2Scale"] = v6h2P * 100 / len

	var dom = `
	<td data-val='city' data-toggle='tooltip' data-placement='top' data-original-title='点击查看{{.university.city}}地区所有高校' class='align-middle classify'  data-city='{{.university.city}}'>{{.university.city}}</td>
	<td data-val='count' class='align-middle'>{{.university.count}}</td>
	<td data-val='supportIpv6Count' class='align-middle'>{{.university.ipv6Count}}</td>
	<td data-val='supportIpv6Scale' class='align-middle'>{{.university.ipv6Scale}}%</td>
	<td data-val='supportIpv6HttpCount' class='align-middle'>{{.university.v6hpCount}}</td>
	<td data-val='supportIpv6HttpScale' class='align-middle'>{{.university.v6hpScale}}%</td>
	<td data-val='supportIpv6HttpsCount' class='align-middle'>{{.university.v6hsCount}}</td>
	<td data-val='supportIpv6HttpsScale' class='align-middle'>{{.university.v6hsScale}}%</td>
	<td data-val='supportIpv6Http2Count' class='align-middle'>{{.university.v6h2Count}}</td>
	<td data-val='supportIpv6Http2Scale' class='align-middle'>{{.university.v6h2Scale}}%</td>
	`

	var buf bytes.Buffer
	t, _ := template.New("dom").Parse(dom)
	t.Execute(&buf, map[string]interface{}{"university": university})
	return buf.String()
}

func cityuniversitydetail(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var city = req.URL.Query().Get("city")
	log.Printf("Method %s RemoteAddr %s User-Agent %s Behavior Load %s's University\n", req.Method, req.RemoteAddr, req.UserAgent(), city)
	var cityUniversityDetails []Site

	if err := db.Table("lable").Join("INNER", "site", "site.id=lable.sid").Where("lable.classify=? and lable.lable=?", "university", city).Desc("site.v6time").Find(&cityUniversityDetails); err != nil {
		panic(err)
	}
	var dom = `<tr class='tag-{{.city}} cityUniversityDetails' style='background: rgb(249, 249, 182)'>
	<td></td>
	<td colspan='9'>
		<table width='100%' class='universityArea'>
			<thead>
			<tr class='table-success'>
				<th scope='col'>高校</th>
				<th scope='col'>IPv6</th>
				<th scope='col'>IPv6 http</th>
				<th scope='col'>IPv6 https</th>
				<th scope='col'>IPv6 h2</th>
			</tr>
			</thead>
			{{range $k,$v := .cityUniversityDetails}}
			<tr>
				<td>{{$v.Desc}}（<a href='http://{{$v.Domain}}'>{{$v.Domain}}</a>）</td>
				<td>{{if $v.IPv6}}{{.IPv6}}{{end}}</td>
				<td>{{if eq $v.V6hp 2}}<button type='button' class='btn btn-outline-success btn-sm'>已支持</button>{{else}}<button type='button' class='btn btn-outline-danger btn-sm'>不支持</button>{{end}}</td>
				<td>{{checkCertificate $v 6}}</td>
				<td>{{if eq $v.V6h2 2}}<button type='button' class='btn btn-outline-success btn-sm'>已支持</button>{{else}}<button type='button' class='btn btn-outline-danger btn-sm'>不支持</button>{{end}}</td>
			</tr>
			{{end}}
		</table>
	</td></tr>`

	t, _ := template.New("dom").Funcs(template.FuncMap{"checkCertificate": checkCertificate}).Parse(dom)
	t.Execute(w, map[string]interface{}{"cityUniversityDetails": cityUniversityDetails, "city": city})
}
