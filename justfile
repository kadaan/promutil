default:
  @just --list

clean:
  @echo "==> Cleanup"
  @rm -rf $DIST_DIR
  @mkdir -p $DIST_DIR

generate:
  @echo "==> Generating code"
  @cd "${DEVBOX_PROJECT_ROOT}/lib/web/ui" && go generate

build: clean generate build-linux build-darwin compress

build-linux: (build-os "linux" "osusergo netgo jsoniter static_build" "" "linux-musl")

build-darwin: (build-os "darwin" "osusergo netgo jsoniter" "-extldflags '-fno-PIC -static'" "macos-none")
  @echo "==> Making universal darwin binary"
  lipo $DIST_DIR/darwin/amd64/promutil $DIST_DIR/darwin/arm64/promutil -create -output $DIST_DIR/darwin/promutil
  rm -rf $DIST_DIR/darwin/amd64
  rm -rf $DIST_DIR/darwin/arm64

build-os os tags ext_ld_flags type: (build-arch os "amd64" "x86_64-" + type tags ext_ld_flags) (build-arch os "arm64" "aarch64-" + type tags ext_ld_flags)

build-arch os arch target tags ext_ld_flags:
  @echo "==> Building {{os}}/{{arch}}"
  GOOS={{os}} GOARCH={{arch}} CC="zig cc -target {{target}}" CXX="zig c++ -target {{target}}" CGO_ENABLED=0 go build \
      -o $DIST_DIR/{{os}}/{{arch}}/promutil \
      -a \
      -ldflags "-s -w {{ext_ld_flags}} -X github.com/kadaan/promutil/version.Version=$VERSION -X github.com/kadaan/promutil/version.Revision=$REVISION -X github.com/kadaan/promutil/version.Branch=$BRANCH -X github.com/kadaan/promutil/version.BuildUser=$USER -X github.com/kadaan/promutil/version.BuildHost=$HOST -X github.com/kadaan/promutil/version.BuildDate=$BUILD_DATE" \
      -tags '{{tags}}' \
      -installsuffix netgo

compress:
  @echo "==> Compressing binaries"
  tar -czf $DIST_DIR/promutil_linux_amd64.tar.gz -C $DIST_DIR/linux/amd64 promutil
  tar -czf $DIST_DIR/promutil_linux_arm64.tar.gz -C $DIST_DIR/linux/arm64 promutil
  tar -czf $DIST_DIR/promutil_darwin_universal.tar.gz -C $DIST_DIR/darwin promutil
