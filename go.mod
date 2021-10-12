module www.velocidex.com/golang/evtx

require (
	github.com/Velocidex/ordereddict v0.0.0-20210502082334-cf5d9045c0d1
	github.com/alecthomas/assert v0.0.0-20170929043011-405dbfeb8e38
	github.com/davecgh/go-spew v1.1.1
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/mattn/go-sqlite3 v1.14.8
	github.com/pkg/errors v0.8.1
	github.com/sebdah/goldie v1.0.0 // indirect
	github.com/stretchr/testify v1.4.0
	golang.org/x/sys v0.0.0-20200116001909-b77594299b42
	golang.org/x/text v0.3.7 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0 // indirect
	www.velocidex.com/golang/binparsergen v0.1.1-0.20201101234514-bbdb29f9ee31
	www.velocidex.com/golang/go-pe v0.1.1-0.20211006062218-8f6d1ad6b2d5
)

// replace www.velocidex.com/golang/go-pe => /home/mic/projects/go-pe/
//replace github.com/Velocidex/ordereddict => /home/mic/projects/ordereddict

go 1.13
