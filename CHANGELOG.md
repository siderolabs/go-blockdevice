## [go-blockdevice 2.0.0](https://github.com/siderolabs/go-blockdevice/releases/tag/v2.0.0) (2024-08-30)

Welcome to the v2.0.0 release of go-blockdevice!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/go-blockdevice/issues.

### Contributors

* Andrey Smirnov
* Dmitry Sharshakov
* Dmitriy Matrenichev

### Changes
<details><summary>20 commits</summary>
<p>

* [`08a7802`](https://github.com/siderolabs/go-blockdevice/commit/08a7802e22eb6dc6540b4311e3bbeb8ab5dc52b1) fix: add support for 'legacy bios bootable' attribute
* [`fa9291f`](https://github.com/siderolabs/go-blockdevice/commit/fa9291f45dd68d7148ba19b8765e2d8084e493bf) feat: bring disk encryption support
* [`bc73f6d`](https://github.com/siderolabs/go-blockdevice/commit/bc73f6d6b2e2bfcbb46f02f0ce93da1d76596c40) fix: drop `ReadFullAt`
* [`c34dfb6`](https://github.com/siderolabs/go-blockdevice/commit/c34dfb6570cbe28cff2b4c3e73591472e8dd06ce) fix: don't ignore error on partition delete
* [`41240c1`](https://github.com/siderolabs/go-blockdevice/commit/41240c1d2f1d6d6b2cc92b47b5d5f228ba1aada2) fix: several fixes for GPT and ZFS
* [`07f736f`](https://github.com/siderolabs/go-blockdevice/commit/07f736fa3c03611c3fd8a8a6bced5ffd88867458) feat: add support for retry-locking the blockdevice
* [`114af20`](https://github.com/siderolabs/go-blockdevice/commit/114af20196847618265af39862b434021f488e25) feat: add device wipe and partition devname
* [`cfdeb03`](https://github.com/siderolabs/go-blockdevice/commit/cfdeb03b051f58d2f2b253c1056ae3f92eea9c52) feat: implement GPT editing
* [`9d8d8e7`](https://github.com/siderolabs/go-blockdevice/commit/9d8d8e7c48c9d647d8dcb141ca64317dec6d0c76) chore: add setters for struct fields
* [`f4a4030`](https://github.com/siderolabs/go-blockdevice/commit/f4a4030394f4b76b214808027509306d853f901f) fix: add `runtime.KeepAlive` to keep alive descriptors
* [`1a51f16`](https://github.com/siderolabs/go-blockdevice/commit/1a51f162a09e3630cd845630f5402b142e86ece2) feat: gather blockdevice information
* [`81b69bf`](https://github.com/siderolabs/go-blockdevice/commit/81b69bf28eaaa53990248df0b803e50be8824cd8) fix: use read full when reading data
* [`3052077`](https://github.com/siderolabs/go-blockdevice/commit/3052077bc67bfcb2d2621707b05e1827861489dd) feat: lock the blockdevice in shared mode when probing
* [`cf51e33`](https://github.com/siderolabs/go-blockdevice/commit/cf51e3318ef39dd6ff453a418cb40bbaf5fe7427) feat: support detection of squashfs and Talos META
* [`da92100`](https://github.com/siderolabs/go-blockdevice/commit/da92100e2f889fcfbd98b9613aba255b527574e4) feat: detect ZFS
* [`3265299`](https://github.com/siderolabs/go-blockdevice/commit/3265299b0192bf1481bc0f8d998359607cf7ddf2) feat: detect Linux swap and LVM2
* [`21c66f8`](https://github.com/siderolabs/go-blockdevice/commit/21c66f8bb4ba47f9e7269cba7822b102680e6ca8) fix: don't probe empty CD drives
* [`a5481f5`](https://github.com/siderolabs/go-blockdevice/commit/a5481f5272f270cc750f22f8e5e728316dc62e06) feat: implement GPT partition discovery
* [`9beb2bd`](https://github.com/siderolabs/go-blockdevice/commit/9beb2bd4036170428aa29557f2995a57aa9979fa) feat: start the blkid work
* [`aa55391`](https://github.com/siderolabs/go-blockdevice/commit/aa553918c96588e43808ddeadeea980df16ea3e6) chore: start the v2 version of the module
</p>
</details>

### Dependency Changes

This release has no dependency changes

