module github.com/yusufarbc/vantage

go 1.25.0

require (
	bitbucket.org/liamstask/goose v0.0.0-20150115234039-8488cc47d90c
	github.com/NYTimes/gziphandler v1.1.1
	github.com/PuerkitoBio/goquery v1.5.0
	github.com/emersion/go-imap v1.0.4
	github.com/emersion/go-message v0.12.0
	github.com/go-sql-driver/mysql v1.5.0
	github.com/gophish/gomail v0.0.0-20200818021916-1f6d0dfd512e
	github.com/gorilla/context v1.1.1
	github.com/gorilla/handlers v1.4.2
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/securecookie v1.1.2
	github.com/gorilla/sessions v1.2.0
	github.com/gorilla/websocket v1.5.0
	github.com/jinzhu/gorm v1.9.12
	github.com/jordan-wright/email v4.0.1-0.20200824153738-3f5bafa1cd84+incompatible
	github.com/jordan-wright/unindexed v0.0.0-20181209214434-78fa79113c0f
	github.com/jung-kurt/gofpdf v1.16.2
	github.com/oschwald/maxminddb-golang v1.6.0
	github.com/sirupsen/logrus v1.9.4
	golang.org/x/crypto v0.51.0
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405
)

require (
	filippo.io/csrf v0.2.1
	github.com/mattn/go-sqlite3 v2.0.1+incompatible
	github.com/shirou/gopsutil/v3 v3.24.5
)

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/andybalholm/cascadia v1.0.0 // indirect
	github.com/emersion/go-sasl v0.0.0-20191210011802-430746ea8b9b // indirect
	github.com/emersion/go-textwrapper v0.0.0-20160606182133-d0e65e56babe // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kylelemons/go-gypsy v0.0.0-20160905020020-08cad365cd28 // indirect
	github.com/lib/pq v1.1.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/ziutek/mymysql v1.5.4 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
)

// jinzhu/gorm v1.9.12 pins the mis-tagged go-sqlite3 v2.0.x+incompatible (which is
// actually old v1-era code). Force a real, current v1.14.x so we build against an
// up-to-date SQLite with its security and bug fixes.
replace github.com/mattn/go-sqlite3 => github.com/mattn/go-sqlite3 v1.14.22
