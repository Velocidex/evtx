module www.velocidex.com/golang/evtx

require (
	github.com/Velocidex/ordereddict v0.0.0-20230909174157-2aa49cc5d11d
	github.com/alecthomas/assert v1.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/hashicorp/golang-lru v1.0.2
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/pkg/errors v0.9.1
	github.com/sebdah/goldie v1.0.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sys v0.22.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	www.velocidex.com/golang/binparsergen v0.1.1-0.20240404114946-8f66c7cf586e
	www.velocidex.com/golang/go-pe v0.1.1-0.20230228112150-ef2eadf34bc3
)

require (
	github.com/Velocidex/json v0.0.0-20220224052537-92f3c0326e5a // indirect
	github.com/Velocidex/pkcs7 v0.0.0-20230220112103-d4ed02e1862a // indirect
	github.com/Velocidex/yaml/v2 v2.2.8 // indirect
	github.com/alecthomas/colour v0.1.0 // indirect
	github.com/alecthomas/repr v0.1.1 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace www.velocidex.com/golang/go-pe => /home/mic/projects/go-pe/
//replace github.com/Velocidex/ordereddict => /home/mic/projects/ordereddict

go 1.20
