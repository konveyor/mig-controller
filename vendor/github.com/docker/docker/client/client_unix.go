<<<<<<< HEAD
// +build linux freebsd openbsd darwin solaris illumos
=======
// +build linux freebsd openbsd netbsd darwin dragonfly
>>>>>>> cbc9bb05... fixup add vendor back

package client // import "github.com/docker/docker/client"

// DefaultDockerHost defines os specific default if DOCKER_HOST is unset
const DefaultDockerHost = "unix:///var/run/docker.sock"

const defaultProto = "unix"
const defaultAddr = "/var/run/docker.sock"
