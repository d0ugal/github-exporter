# Changelog

## [0.3.0](https://github.com/d0ugal/github-exporter/compare/v0.2.0...v0.3.0) (2025-11-01)


### Features

* add OpenTelemetry tracing configuration support ([015f1da](https://github.com/d0ugal/github-exporter/commit/015f1da2495b8bf088e92eda973ba8bd7deaac8e))
* add tracing configuration support ([4900ec4](https://github.com/d0ugal/github-exporter/commit/4900ec4b7c700f8175278ddd378d4a86d84dfe7b))
* **ci:** add auto-format workflow ([09597cd](https://github.com/d0ugal/github-exporter/commit/09597cd30ef9acc2c83a05351f3af739ce18d4b7))
* **ci:** add auto-format workflow ([3b78328](https://github.com/d0ugal/github-exporter/commit/3b78328387dd18a37122a32bb32d56e1c0b6b75f))
* enhance tracing support with detailed spans ([d05a415](https://github.com/d0ugal/github-exporter/commit/d05a41589ba93d686e75be533ced9cd0d50fa6f6))
* enhance tracing support with detailed spans ([ee533b6](https://github.com/d0ugal/github-exporter/commit/ee533b60e068c97234f2273d51f9dd5d1b63351f))
* integrate OpenTelemetry tracing into collector ([fbd3f48](https://github.com/d0ugal/github-exporter/commit/fbd3f48f0dcfb2fecfa1210e488067d472e600f2))
* integrate OpenTelemetry tracing into collector ([6f797b2](https://github.com/d0ugal/github-exporter/commit/6f797b255f4909965b02d715d30df8dd3a155dca))
* trigger CI after auto-format workflow completes ([ee8e815](https://github.com/d0ugal/github-exporter/commit/ee8e815d3fe4e27fb6580097733fff5e343c11bc))


### Bug Fixes

* add enhanced panic prevention with 404 handling ([8e23aa8](https://github.com/d0ugal/github-exporter/commit/8e23aa878957e581b65ad99d002111b1af11289e))
* add enhanced panic prevention with 404 handling ([5aafa12](https://github.com/d0ugal/github-exporter/commit/5aafa122bb9907e8e24e84e2c2648f3b906fedc3))
* check resp nil before accessing StatusCode ([d85b692](https://github.com/d0ugal/github-exporter/commit/d85b69285d92e08a922c7daaf307294e5d6026e7))
* correct PR count calculation to use actual per_page value ([ed0e222](https://github.com/d0ugal/github-exporter/commit/ed0e222b2f7517d7faebb67a74ae0ffb88d7214c))
* correct PR count calculation to use actual per_page value ([dc04b41](https://github.com/d0ugal/github-exporter/commit/dc04b418a82784e6ad764159c16a8bb70e909e39))
* prevent org label panic and add pagination support ([c72ce31](https://github.com/d0ugal/github-exporter/commit/c72ce31be3a4db34ec9f820d596fdb6cdf870654))
* prevent org label panic and add pagination support ([f9c87c6](https://github.com/d0ugal/github-exporter/commit/f9c87c69de4af1d123b67664aa1c561d10f749c6))
* prevent panic from missing org label in metrics ([cbb6900](https://github.com/d0ugal/github-exporter/commit/cbb690074dcb0bb93817a02f959aa3bef7d4fe17))
* prevent panic from missing org label in metrics ([ccadec7](https://github.com/d0ugal/github-exporter/commit/ccadec72acd93ade15d43530a55c612336f54649))
* prevent panic when org fetch fails ([b001288](https://github.com/d0ugal/github-exporter/commit/b001288390317e032fcf1b61fcd0679534d364d3))
* prevent panic when org fetch fails (404) ([07f3361](https://github.com/d0ugal/github-exporter/commit/07f33619d736902594cee68f7737e549cd07cce8))
* resolve staticcheck linting issues in tests ([5b6e622](https://github.com/d0ugal/github-exporter/commit/5b6e6220c3e514834cde719b00b669bdfa6a062f))
* update google.golang.org/genproto/googleapis/api digest to ab9386a ([#53](https://github.com/d0ugal/github-exporter/issues/53)) ([581caf7](https://github.com/d0ugal/github-exporter/commit/581caf747570c09e588a5e42298e5b8a3b1eb5df))
* update google.golang.org/genproto/googleapis/rpc digest to ab9386a ([e58dd77](https://github.com/d0ugal/github-exporter/commit/e58dd7797e043394bb7157a745a718ed0d82d8c1))
* update google.golang.org/genproto/googleapis/rpc digest to ab9386a ([e0aa9e5](https://github.com/d0ugal/github-exporter/commit/e0aa9e5d1dd2708191ccbfbb9e4de401c3e8015e))
* update module github.com/d0ugal/promexporter to v1.6.1 ([#40](https://github.com/d0ugal/github-exporter/issues/40)) ([c6370e4](https://github.com/d0ugal/github-exporter/commit/c6370e4d5811569f07e8b923bb4bf747081f4a11))
* update module github.com/d0ugal/promexporter to v1.7.0 ([#58](https://github.com/d0ugal/github-exporter/issues/58)) ([d435c26](https://github.com/d0ugal/github-exporter/commit/d435c2641987352962581e01910197f0681e0a0e))
* update module github.com/d0ugal/promexporter to v1.7.1 ([#59](https://github.com/d0ugal/github-exporter/issues/59)) ([85eac2f](https://github.com/d0ugal/github-exporter/commit/85eac2fc8d4922fdf9f899045e5f34d4515997e9))
* update module github.com/gabriel-vasile/mimetype to v1.4.11 ([#49](https://github.com/d0ugal/github-exporter/issues/49)) ([0873f60](https://github.com/d0ugal/github-exporter/commit/0873f6035a4d73e0217635951d5d598916cc6596))
* update module github.com/prometheus/common to v0.67.2 ([#38](https://github.com/d0ugal/github-exporter/issues/38)) ([f785475](https://github.com/d0ugal/github-exporter/commit/f785475e09a847427f14581cbdc5abaf251c9496))
* use GitHub Search API for exact PR count instead of estimation ([dbc9a0e](https://github.com/d0ugal/github-exporter/commit/dbc9a0ef90b3f7266a2d77e8bf6b3ffcc45816d9))

## [0.2.0](https://github.com/d0ugal/github-exporter/compare/v0.1.2...v0.2.0) (2025-10-28)


### Features

* add build status monitoring for branches ([65f8bf3](https://github.com/d0ugal/github-exporter/commit/65f8bf32bf63f63786a69769ecb6a8b1a17dcf3e))
* add build status monitoring for branches ([a8a9242](https://github.com/d0ugal/github-exporter/commit/a8a9242d52c65dbe157526ee61d41f9e2b0d3f76))
* add dev-tag Makefile target ([4819fa7](https://github.com/d0ugal/github-exporter/commit/4819fa76f487e8fc9b3ce3e33c9344a02465d330))


### Bug Fixes

* add wildcard repository support for build status metrics ([c45dcf1](https://github.com/d0ugal/github-exporter/commit/c45dcf160f616a0e8a883825d0cf6b4e032a8859))
* **ci:** use Makefile for linting instead of golangci-lint-action ([6e49b7b](https://github.com/d0ugal/github-exporter/commit/6e49b7ba53c6bef53551f2525c9bfc1b3269a4f6))
* **ci:** use Makefile for linting instead of golangci-lint-action ([b4fbf2e](https://github.com/d0ugal/github-exporter/commit/b4fbf2e1e23e1dde8f70ef85ca38d5bd28e0d750))
* correct all GitHubAPIErrorsTotal label mappings ([1ad41de](https://github.com/d0ugal/github-exporter/commit/1ad41de4bdddec7946aa9ac0f26f32a1585117f9))
* correct GitHubAPIErrorsTotal label mapping ([395eb36](https://github.com/d0ugal/github-exporter/commit/395eb36a2c8dc7907ba0ee62c4819156686a4ef6))
* correct GitHubReposInfo and related metrics label mapping ([d5b3758](https://github.com/d0ugal/github-exporter/commit/d5b3758fae227966d4bee608dc2a9d3dbad5c61e))
* correct label mapping for GitHub API error metrics ([ab9b1b8](https://github.com/d0ugal/github-exporter/commit/ab9b1b803da2cff93752f6e67f09b07bdfa340e0))
* correct label mapping for GitHub API error metrics ([ac5503b](https://github.com/d0ugal/github-exporter/commit/ac5503b16e247f0edb0e3834a90dc54801f1db2c))
* lint ([fb7d70c](https://github.com/d0ugal/github-exporter/commit/fb7d70cb7019bafa739ba61bf56db4c48b1036c1))
* missed one WithLabelValues call ([2f893c3](https://github.com/d0ugal/github-exporter/commit/2f893c32932b00d9301f91ff42f9b08a70d6576a))
* update module github.com/bytedance/sonic to v1.14.2 ([#36](https://github.com/d0ugal/github-exporter/issues/36)) ([bdb72a5](https://github.com/d0ugal/github-exporter/commit/bdb72a5bf16172ca7b935bc80421ac1274fbe1c7))
* update module github.com/bytedance/sonic/loader to v0.4.0 ([#32](https://github.com/d0ugal/github-exporter/issues/32)) ([0f411a5](https://github.com/d0ugal/github-exporter/commit/0f411a5c74aa515479ad6ebfd9f95c6fb7801056))
* update module github.com/ugorji/go/codec to v1.3.1 ([#37](https://github.com/d0ugal/github-exporter/issues/37)) ([d5a4096](https://github.com/d0ugal/github-exporter/commit/d5a4096b1d970c8f40272e9e296a6dba18702b09))

## [0.1.2](https://github.com/d0ugal/github-exporter/compare/v0.1.1...v0.1.2) (2025-10-26)


### Bug Fixes

* add internal version package and update version handling ([93edeb9](https://github.com/d0ugal/github-exporter/commit/93edeb9a91dfa7197da21756e08da14ccb01650d))
* add internal version package and update version handling ([6cb40de](https://github.com/d0ugal/github-exporter/commit/6cb40deef7388120edb7afe10e0f41402e361914))
* update module github.com/d0ugal/promexporter to v1.5.0 ([a118ea7](https://github.com/d0ugal/github-exporter/commit/a118ea77b63b7ce15348914cfdc125a61234edb1))
* update module github.com/d0ugal/promexporter to v1.5.0 ([938472b](https://github.com/d0ugal/github-exporter/commit/938472ba3089a1b62f413c3cc5e50516faf0e5ce))
* update module github.com/prometheus/procfs to v0.19.1 ([404aa8e](https://github.com/d0ugal/github-exporter/commit/404aa8e3eca7da5847f2083bcf3922d5aa967cf9))
* update module github.com/prometheus/procfs to v0.19.1 ([39005b4](https://github.com/d0ugal/github-exporter/commit/39005b4684b52ef68998a261255c7691dfe8a784))
* use WithVersionInfo to pass version info to promexporter ([a6bc2db](https://github.com/d0ugal/github-exporter/commit/a6bc2dbcef936535d4bd04fc0e002fa9c93d29f8))

## [0.1.1](https://github.com/d0ugal/github-exporter/compare/v0.1.0...v0.1.1) (2025-10-25)


### Bug Fixes

* update module github.com/d0ugal/promexporter to v1.4.1 ([1e08dd6](https://github.com/d0ugal/github-exporter/commit/1e08dd6e13023e0348f595b478c189548262ada1))
* update module github.com/d0ugal/promexporter to v1.4.1 ([e2e80bb](https://github.com/d0ugal/github-exporter/commit/e2e80bb4a44b4093d2ab630d0ecf7431152c7b99))
* update module github.com/prometheus/procfs to v0.19.0 ([43396b0](https://github.com/d0ugal/github-exporter/commit/43396b0380f53083b86f392fe2e1b5504271dd58))
* update module github.com/prometheus/procfs to v0.19.0 ([703e33f](https://github.com/d0ugal/github-exporter/commit/703e33faeacff6929dd0ee4c0c4caefc415a7ba6))

## [0.1.0](https://github.com/d0ugal/github-exporter/compare/v0.0.1...v0.1.0) (2025-10-25)


### Features

* add pull request metrics and repository info with archived status ([6e7e806](https://github.com/d0ugal/github-exporter/commit/6e7e806ccc057da287e4fad00067d3260cea4fd9))
* add wildcard support for repositories ([2eff518](https://github.com/d0ugal/github-exporter/commit/2eff518c0df4daf285afc7ee2d51a6e8e48fd037))
* initial github-exporter implementation with intelligent rate limiting ([2da22eb](https://github.com/d0ugal/github-exporter/commit/2da22eb938daf64d40376839de81b9db09510a62))
* update promexporter to v1.4.0 ([3cf927e](https://github.com/d0ugal/github-exporter/commit/3cf927e8474afbbf8163a7509693e6f30e4e7c87))
* update promexporter to v1.4.0 ([87e25bc](https://github.com/d0ugal/github-exporter/commit/87e25bce46ad64dc1eea902b071f2f66e9985202))


### Bug Fixes

* update module github.com/d0ugal/promexporter to v1.0.2 ([bb56609](https://github.com/d0ugal/github-exporter/commit/bb56609021ab884452e8823b858df6225fd297fe))
* update module github.com/d0ugal/promexporter to v1.0.2 ([20c1791](https://github.com/d0ugal/github-exporter/commit/20c1791efd617364e3684e80a8726ebbba0d6242))
* update module github.com/d0ugal/promexporter to v1.1.0 ([83ed3c0](https://github.com/d0ugal/github-exporter/commit/83ed3c004d4cbe65bbdd1be3baa041a757b5d19d))
* update module github.com/d0ugal/promexporter to v1.1.0 ([d9b85ce](https://github.com/d0ugal/github-exporter/commit/d9b85ce15fda622746fed32e8f2ec2e4054e6a6f))
* update module github.com/d0ugal/promexporter to v1.3.1 ([#9](https://github.com/d0ugal/github-exporter/issues/9)) ([8a8166d](https://github.com/d0ugal/github-exporter/commit/8a8166d3ca269248ac8dce826b1d43f2ee2a3ca7))
* update module github.com/prometheus/procfs to v0.18.0 ([c32e50b](https://github.com/d0ugal/github-exporter/commit/c32e50b86245962ce86f1bf8d8b4536ea0827293))
* update module github.com/prometheus/procfs to v0.18.0 ([8a473e4](https://github.com/d0ugal/github-exporter/commit/8a473e4df55d03343767c0261e6dd8ecee629c03))
