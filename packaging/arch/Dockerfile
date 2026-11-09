FROM archlinux:base-devel

ARG VERSION
ARG SOURCE_DATE_EPOCH
ENV PKGDEST=/work/out \
    PKGEXT=.pkg.tar.zst

WORKDIR /work
COPY PKGBUILD LICENSE NOTICE clara-code-linux-* ./
RUN useradd -m builder && mkdir out && chown -R builder:builder /work

USER builder
RUN set -eu; \
    test "$(./clara-code-linux-amd64 --version)" = "Clara Code $VERSION"; \
    for arch in x86_64 aarch64; do \
      case "$arch" in \
        x86_64) binary=clara-code-linux-amd64 ;; \
        aarch64) binary=clara-code-linux-arm64 ;; \
      esac; \
      cp /etc/makepkg.conf makepkg.conf; \
      printf '\nCARCH=%s\n' "$arch" >> makepkg.conf; \
      PACKAGE_VERSION="$VERSION" PACKAGE_ARCH="$arch" PACKAGE_BINARY="/work/$binary" \
        makepkg --config /work/makepkg.conf --noconfirm --nodeps --clean; \
      package="$PKGDEST/clara-code-$VERSION-1-$arch.pkg.tar.zst"; \
      bsdtar -xOf "$package" .PKGINFO > package-info; \
      grep -Fx 'pkgname = clara-code' package-info; \
      grep -Fx "pkgver = $VERSION-1" package-info; \
      grep -Fx "arch = $arch" package-info; \
      bsdtar -xOf "$package" usr/bin/clara-code > packaged-binary; \
      cmp "$binary" packaged-binary; \
    done
