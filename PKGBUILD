# Maintainer: Yousef El-Darsh <yousef.eldarsh@gmail.com>

pkgname="fabric-cli-git"
pkgdesc="an alternative cli for fabric"
url="https://github.com/Fabric-Development/fabric-cli"
pkgrel=1
pkgver=0.0.2
license=('AGPL3')
provides=("fabric-cli")
arch=('x86_64')
source=("git+${url}.git")
depends=('go')
makedepends=('meson' 'ninja')
conflicts=('fabric-cli')

sha256sums=('SKIP')

build() {
    cd "$srcdir/$pkgname"
    arch-meson build
}

package() {
    cd "$srcdir/$pkgname/build"
    meson install --destdir "$pkgdir"
}
