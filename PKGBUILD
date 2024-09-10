# Maintainer: Yousef El-Darsh <yousef.eldarsh@gmail.com>

pkgname="fabric-cli"
pkgdesc="an alternative cli for fabric"
url="https://github.com/Fabric-Development/fabric-cli"
pkgver=0.0.2
pkgrel=1
arch=('x86_64')
license=('AGPL3')
depends=('go')
makedepends=('meson' 'ninja')
source=("git+${url}.git")
sha256sums=('SKIP')

build() {
    cd "$srcdir/$pkgname"
    arch-meson build
}

package() {
    cd "$srcdir/$pkgname/build"
    meson install --destdir "$pkgdir"
}
