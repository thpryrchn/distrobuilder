module github.com/lxc/distrobuilder

go 1.13

replace (
	github.com/codegangsta/cli => github.com/urfave/cli v1.22.5
	github.com/rootless-containers/proto v0.1.0 => github.com/rootless-containers/proto/go-proto v0.0.0-20210829182612-43763522b879
)

exclude (
	github.com/klauspost/compress v1.13.6
	github.com/rootless-containers/proto v0.1.0
	github.com/rootless-containers/proto/go-proto v0.0.0-20210921234734-69430b6543fb
)

require (
	github.com/Microsoft/hcsshim v0.8.22 // indirect
	github.com/antchfx/xpath v1.2.0 // indirect
	github.com/apex/log v1.9.0 // indirect
	github.com/containerd/continuity v0.2.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/flosch/pongo2 v0.0.0-20200913210552-0d938eb266f3
	github.com/fsouza/go-dockerclient v1.7.4 // indirect
	github.com/gobuffalo/logger v1.0.4 // indirect
	github.com/gobuffalo/packr/v2 v2.8.1
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/heroku/docker-registry-client v0.0.0-20190909225348-afc9e1acc3d5 // indirect
	github.com/karrick/godirwalk v1.16.1 // indirect
	github.com/klauspost/compress v1.13.5 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/lxc/lxd v0.0.0-20210924110146-2a036b5dcae8
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mudler/docker-companion v0.4.6-0.20201209184016-2d26fc9143d4
	github.com/openSUSE/umoci v0.4.5 // indirect
	github.com/opencontainers/runc v1.0.2 // indirect
	github.com/rootless-containers/proto/go-proto v0.0.0-20210829182612-43763522b879 // indirect
	github.com/spf13/cobra v1.2.1
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli v1.22.5 // indirect
	github.com/vbatts/go-mtree v0.5.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.1
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519 // indirect
	golang.org/x/net v0.0.0-20210924054057-cf34111cab4d // indirect
	golang.org/x/sys v0.0.0-20210923061019-b8560ed6a9b7
	golang.org/x/term v0.0.0-20210916214954-140adaaadfaf // indirect
	gopkg.in/antchfx/htmlquery.v1 v1.2.2
	gopkg.in/flosch/pongo2.v3 v3.0.0-20141028000813-5e81b817a0c4
	gopkg.in/yaml.v2 v2.4.0
)
