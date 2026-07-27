# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

### [16.60.4](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.60.3...v16.60.4) (2026-07-27)

### [16.60.3](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.60.2...v16.60.3) (2026-07-27)

### [16.60.2](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.60.1...v16.60.2) (2026-07-23)

### [16.60.1](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.60.0...v16.60.1) (2026-07-22)

## [16.60.0](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.8...v16.60.0) (2026-07-20)


### Features

* **firestore:** persist pending-input source display metadata ([#14](https://github.com/Ripixel-Studio/fitglue-server/issues/14)) ([5560399](https://github.com/Ripixel-Studio/fitglue-server/commit/55603990f6b57b2bd90e7a607362acd71eae71c1))

### [16.59.8](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.7...v16.59.8) (2026-07-20)

### [16.59.7](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.6...v16.59.7) (2026-07-20)

### [16.59.6](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.5...v16.59.6) (2026-07-20)

### [16.59.5](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.4...v16.59.5) (2026-07-19)

### [16.59.4](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.3...v16.59.4) (2026-07-18)

### [16.59.3](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.2...v16.59.3) (2026-07-18)

### [16.59.2](https://github.com/Ripixel-Studio/fitglue-server/compare/v16.59.1...v16.59.2) (2026-07-18)


### Bug Fixes

* **ci:** point web-rebuild trigger at the renamed Ripixel-Studio/fitglue-web project ([0c7ce40](https://github.com/Ripixel-Studio/fitglue-server/commit/0c7ce40873571482888f7d4f99e9fd239f0115a6))
* **showcase:** stop public showcases from losing activity data after 7 days ([8fd1447](https://github.com/Ripixel-Studio/fitglue-server/commit/8fd144746619bc9e2460e4b6d14d3532fe9a3c8e))

### [16.59.1](https://github.com/FitGlue/server/compare/v16.59.0...v16.59.1) (2026-07-03)


### Bug Fixes

* **destination:** correctly suppress/lighten notifications on re-runs ([dcb8726](https://github.com/FitGlue/server/commit/dcb87267c3a534af9fa8767faf690055cb0f33a5))
* **destination:** don't duplicate description on repeated Update() calls ([f67bf25](https://github.com/FitGlue/server/commit/f67bf2515728791774296f61d17bce0f8018f40a))
* **firestore:** add missing collection-group index for pipeline_runs.created_at ([8e575ae](https://github.com/FitGlue/server/commit/8e575ae2a848895e145ac6435e339ebf0d30d738))
* **pipeline:** replay heart rate stream on resume instead of re-fetching ([c48a512](https://github.com/FitGlue/server/commit/c48a5128e371e831d57d0ae9b9f1611bdf8ba34a))

## [16.59.0](https://github.com/FitGlue/server/compare/v16.58.1...v16.59.0) (2026-07-01)


### Features

* **activity:** add real pagination to recent public roundups ([e8d3097](https://github.com/FitGlue/server/commit/e8d3097273235806a0cad4ac648d210a8cb1700c))

### [16.58.1](https://github.com/FitGlue/server/compare/v16.58.0...v16.58.1) (2026-07-01)


### Bug Fixes

* **roundups:** order showcase roundups by last date covered ([aced707](https://github.com/FitGlue/server/commit/aced70780f39af79adfbc3b20ad1952c1466e791))

## [16.58.0](https://github.com/FitGlue/server/compare/v16.57.4...v16.58.0) (2026-06-22)


### Features

* **cors:** add public CORS middleware for unauthenticated API access ([b04ef39](https://github.com/FitGlue/server/commit/b04ef3911261c4ef1357bc9e8ece89b4e4545103))

### [16.57.4](https://github.com/FitGlue/server/compare/v16.57.3...v16.57.4) (2026-06-22)


### Bug Fixes

* **hdrop:** strip leading prose before JSON object in export ([c086ebb](https://github.com/FitGlue/server/commit/c086ebbfea5b91e1b2185002a601a211c714328d))

### [16.57.3](https://github.com/FitGlue/server/compare/v16.57.2...v16.57.3) (2026-06-20)

### [16.57.2](https://github.com/FitGlue/server/compare/v16.57.1...v16.57.2) (2026-06-20)


### Bug Fixes

* **muscle-heatmap:** skip section when no muscle groups have volume ([486a284](https://github.com/FitGlue/server/commit/486a284c95c12fa32e332641bb36b4d426b9c325))
* **pace-summary:** derive real per-km splits from record stream ([891cb55](https://github.com/FitGlue/server/commit/891cb5505c4d2e5cf004ea951474ca019fe1832c))

### [16.57.1](https://github.com/FitGlue/server/compare/v16.57.0...v16.57.1) (2026-06-19)


### Bug Fixes

* **admin:** populate run user_id, resolve identity from Firebase, add pipeline config endpoints ([3447aba](https://github.com/FitGlue/server/commit/3447aba653d9bcd6de03de270eb317e763298e8a))

## [16.57.0](https://github.com/FitGlue/server/compare/v16.56.3...v16.57.0) (2026-06-19)


### Features

* **admin:** expand admin gateway with 360° user detail, ops, billing and audit ([1aa50f2](https://github.com/FitGlue/server/commit/1aa50f24b829be7f0489ea29d7ba965733a18e20))

### [16.56.3](https://github.com/FitGlue/server/compare/v16.56.2...v16.56.3) (2026-06-19)


### Bug Fixes

* **admin:** correct platform stats and harden cross-user pipeline-run listing ([7f76600](https://github.com/FitGlue/server/commit/7f766009337a565460fc491295cc7c597cf87a33))

### [16.56.2](https://github.com/FitGlue/server/compare/v16.56.1...v16.56.2) (2026-06-19)

### [16.56.1](https://github.com/FitGlue/server/compare/v16.56.0...v16.56.1) (2026-06-18)


### Bug Fixes

* **destination:** let targeted reposts bypass the already-uploaded guard ([a781c5b](https://github.com/FitGlue/server/commit/a781c5b9284f01dd1ea9b73be0edcd78ee840a10))
* **enricher:** restore and persist typed enrichments during pipeline resume ([514e1f4](https://github.com/FitGlue/server/commit/514e1f4432b35597364a361e92dfc9f1842fdf68))
* **hdrop:** handle string numeric fields and null timeseries values ([6e79482](https://github.com/FitGlue/server/commit/6e794828b8917a7a4f48f8c9195faa8a33ebab5d))
* **pipeline:** prevent infinite Pub/Sub retries on permanent enricher failures ([cbf99a8](https://github.com/FitGlue/server/commit/cbf99a85814e63a64c8c1d8f7320b88bc589f69b))
* **pipeline:** skip pipeline re-run when dismissing non-blocking input ([b88ff42](https://github.com/FitGlue/server/commit/b88ff4286087e7615d012ee8db4af780084f4a2b))
* **pipeline:** store pipeline-run updated_at as a proto Timestamp ([32075e3](https://github.com/FitGlue/server/commit/32075e3bf33bc0d7ce38253226cd9e23c3759770))

## [16.56.0](https://github.com/FitGlue/server/compare/v16.55.3...v16.56.0) (2026-06-17)


### Features

* **enricher:** add location-pinner to map titles to real places ([af80a28](https://github.com/FitGlue/server/commit/af80a28d41adb64c8b448f1e22d64ecb33e9705b))

### [16.55.3](https://github.com/FitGlue/server/compare/v16.55.2...v16.55.3) (2026-06-17)


### Bug Fixes

* **roundup-ai:** increase timeout and output token limit for summary generation ([24dc695](https://github.com/FitGlue/server/commit/24dc6957048256150bde1c9a0738c345bb433285))

### [16.55.2](https://github.com/FitGlue/server/compare/v16.55.1...v16.55.2) (2026-06-17)

### [16.55.1](https://github.com/FitGlue/server/compare/v16.55.0...v16.55.1) (2026-06-17)


### Bug Fixes

* **destination:** prevent duplicate creates from concurrent pending-input resumes ([654714e](https://github.com/FitGlue/server/commit/654714ebcf4ba76d417ab0494093cb9abf38039f))
* **fitbit-hr:** anchor HR to GPS by absolute time and re-arm lag retry on resume ([34b80b7](https://github.com/FitGlue/server/commit/34b80b7c9e2d60071984e484dfd39f7cfcb29340))

## [16.55.0](https://github.com/FitGlue/server/compare/v16.54.0...v16.55.0) (2026-06-17)


### Features

* **roundup:** record source session for each single-session peak ([551f87f](https://github.com/FitGlue/server/commit/551f87f6458568257b7e49760fc211bdd41132ea))


### Bug Fixes

* **activity:** skip undecodable pipeline runs instead of failing the list ([1aaee35](https://github.com/FitGlue/server/commit/1aaee3577cfcf3800bac43815ff657a8a478100c))
* **roundup:** attribute PR callout to the right session, not the day ([cdb653c](https://github.com/FitGlue/server/commit/cdb653c6446d169662d099f16e12d989838d8165))

## [16.54.0](https://github.com/FitGlue/server/compare/v16.53.0...v16.54.0) (2026-06-16)


### Features

* **roundup:** carry showcase_id on wall photos and routes ([afd17e3](https://github.com/FitGlue/server/commit/afd17e3cf7d3b0e00c1771310f93e04a6ed58c91))

## [16.53.0](https://github.com/FitGlue/server/compare/v16.52.0...v16.53.0) (2026-06-16)


### Features

* **activity:** add data export functionality with job management ([b70cc7c](https://github.com/FitGlue/server/commit/b70cc7c18e52364843e972a4bb9c90b1326381eb))

## [16.52.0](https://github.com/FitGlue/server/compare/v16.51.0...v16.52.0) (2026-06-16)


### Features

* add GetShowcaseRoundupViewStats RPC and related functionality ([a276786](https://github.com/FitGlue/server/commit/a276786244f504e2ef4d590bd0596ea2476bca99))

## [16.51.0](https://github.com/FitGlue/server/compare/v16.50.1...v16.51.0) (2026-06-16)


### Features

* **notifications:** emit deep-link path for mobile push navigation ([313c874](https://github.com/FitGlue/server/commit/313c87420e6b60f178911c44ec7ee0538bd9f1e9))
* **showcase:** add privacy-preserving view & visitor metrics ([1f42c5f](https://github.com/FitGlue/server/commit/1f42c5f622184856f016c667f8c5f0fb1b531c74))


### Bug Fixes

* **destination:** stop duplicate sync notifications on non-blocking input updates ([a9a0c75](https://github.com/FitGlue/server/commit/a9a0c754f311572ac3ecfa7b88b44ce2da623218))
* **notification:** resolve recipient email from Firebase Auth when user doc is empty ([4f93f95](https://github.com/FitGlue/server/commit/4f93f9514875d1a36c45bf39b78b05c091351283))

### [16.50.1](https://github.com/FitGlue/server/compare/v16.50.0...v16.50.1) (2026-06-16)

## [16.50.0](https://github.com/FitGlue/server/compare/v16.49.0...v16.50.0) (2026-06-16)


### Features

* **roundup:** aggregate enrichment data for new roundup sections ([f1cf530](https://github.com/FitGlue/server/commit/f1cf530034ca5aa3c9647c9c8dd9c1af281c078f))


### Bug Fixes

* **activity:** set ARTIFACT_BUCKET env for activity Cloud Run service ([1a27603](https://github.com/FitGlue/server/commit/1a27603e91db466a7ef0ed27f587142b9c482494))

## [16.49.0](https://github.com/FitGlue/server/compare/v16.48.0...v16.49.0) (2026-06-15)


### Features

* **roundup:** collect photos and GPS routes for media sections ([4e07cdf](https://github.com/FitGlue/server/commit/4e07cdfd767e2d94d5ef94da739715d30b6c2d50))

## [16.48.0](https://github.com/FitGlue/server/compare/v16.47.0...v16.48.0) (2026-06-15)


### Features

* **roundup:** add callout activities and consistency calendar data ([39d0e49](https://github.com/FitGlue/server/commit/39d0e49308d010711c14382afbb6c6115e16a919))
* **storage:** add IAM member for api-client to access artifacts bucket ([67e1ef8](https://github.com/FitGlue/server/commit/67e1ef837a3f2587932741133601292dcd1065e2))

## [16.47.0](https://github.com/FitGlue/server/compare/v16.46.0...v16.47.0) (2026-06-15)


### Features

* **showcase:** add AI-generated roundup summary via Gemini ([6e83484](https://github.com/FitGlue/server/commit/6e83484aaa3fb5e87960f288827d14cf59763831))

## [16.46.0](https://github.com/FitGlue/server/compare/v16.45.2...v16.46.0) (2026-06-15)


### Features

* **roundup:** add recompute endpoint and highlight stats ([091631c](https://github.com/FitGlue/server/commit/091631cec565b61b179015b376e23a5643896dc5))


### Bug Fixes

* **roundup:** add RecomputeRoundup to mock ActivityServiceClient ([cd0e790](https://github.com/FitGlue/server/commit/cd0e790a5bd1053368729a6e2f480cae6097e9db))

### [16.45.2](https://github.com/FitGlue/server/compare/v16.45.1...v16.45.2) (2026-06-15)


### Bug Fixes

* **export:** update SignedURL parameters to remove content type and length for improved functionality ([d73a761](https://github.com/FitGlue/server/commit/d73a7617edb47645ccfe56ad13feb267ae06bf1e))

### [16.45.1](https://github.com/FitGlue/server/compare/v16.45.0...v16.45.1) (2026-06-15)


### Bug Fixes

* **ai_companion:** increase MaxOutputTokens for improved summary generation ([72900f2](https://github.com/FitGlue/server/commit/72900f286ebb438301fdd436bc25de1c30064b6d))

## [16.45.0](https://github.com/FitGlue/server/compare/v16.44.1...v16.45.0) (2026-06-15)


### Features

* **strava:** add idempotency guard to Create method to prevent duplicate uploads ([9adfb36](https://github.com/FitGlue/server/commit/9adfb3641e2e5c8fbaaa78b37cde8a80c0e5bc88))


### Bug Fixes

* **destinations:** add idempotency guards to Strava and Hevy uploaders ([2c88dd8](https://github.com/FitGlue/server/commit/2c88dd87a86f33077994243d8490ba9935f6a2e6))

### [16.44.1](https://github.com/FitGlue/server/compare/v16.44.0...v16.44.1) (2026-06-15)


### Bug Fixes

* **showcase:** add idempotency guard to Create to prevent Pub/Sub redelivery duplicates ([6bee6c5](https://github.com/FitGlue/server/commit/6bee6c59cff268a5399d3569b2fd989b8ee8fe4e))

## [16.44.0](https://github.com/FitGlue/server/compare/v16.43.2...v16.44.0) (2026-06-14)


### Features

* **showcase:** add fallback method to retrieve showcased activity by pipeline execution ID ([6021806](https://github.com/FitGlue/server/commit/6021806ea9920eb2e43c096e730e7dba546c1b5d))


### Bug Fixes

* **tests:** add GetShowcasedActivityByPipelineExecutionId stub to local test mocks ([b39c29b](https://github.com/FitGlue/server/commit/b39c29bcda85acd291d19ec0ae13289a14677724))

### [16.43.2](https://github.com/FitGlue/server/compare/v16.43.1...v16.43.2) (2026-06-10)


### Bug Fixes

* **mobile-sync:** implement handleMobileSync to publish activities to pipeline ([ccb2682](https://github.com/FitGlue/server/commit/ccb2682732ca2583a6b2c3d7bcf2ad56d6dad473))

### [16.43.1](https://github.com/FitGlue/server/compare/v16.43.0...v16.43.1) (2026-06-10)


### Bug Fixes

* **infra:** grant serviceAccountTokenCreator to cr-api-client-sa ([6c01d1e](https://github.com/FitGlue/server/commit/6c01d1e4acb5870b9ed49c14789aa9752bc7f762))

## [16.43.0](https://github.com/FitGlue/server/compare/v16.42.0...v16.43.0) (2026-06-09)


### Features

* **api-client:** add web-auth-token endpoint for mobile WebView auth bridge ([9805310](https://github.com/FitGlue/server/commit/9805310d29d13d47ce6e7c5786d5ce8466e021d3))

## [16.42.0](https://github.com/FitGlue/server/compare/v16.41.0...v16.42.0) (2026-06-09)


### Features

* **pipeline:** upgrade Gemini model version to 2.5-flash in AI providers ([558e7e4](https://github.com/FitGlue/server/commit/558e7e45dfaa498acd05fe4d06abe2e4065fa239))


### Bug Fixes

* **registry:** mark hdrop enricher as supportsNonBlocking ([c631644](https://github.com/FitGlue/server/commit/c63164469651f4ec6ab64ff83cf64c9a1274f691))

## [16.41.0](https://github.com/FitGlue/server/compare/v16.40.2...v16.41.0) (2026-06-09)


### Features

* **pipeline:** add non_blocking field to enrichers in Pipeline converters ([097b552](https://github.com/FitGlue/server/commit/097b55271b6e769edee1231ca92f2e95f32bec4e))
* **registry:** add hDrop Sweat Analysis enricher manifest ([8f497d7](https://github.com/FitGlue/server/commit/8f497d78bac75616c33b651e7cb46cdb5b012547))

### [16.40.2](https://github.com/FitGlue/server/compare/v16.40.1...v16.40.2) (2026-06-09)


### Bug Fixes

* **platform-stats:** use existing CG composite index for pipeline_runs COUNT ([3635e55](https://github.com/FitGlue/server/commit/3635e5598a64bca888285f3dd7c619bfca5086bf))

### [16.40.1](https://github.com/FitGlue/server/compare/v16.40.0...v16.40.1) (2026-06-09)


### Bug Fixes

* **platform-stats:** count synced pipeline_runs for activitiesBoostedCount ([96b69ec](https://github.com/FitGlue/server/commit/96b69ec317b784e69c139dcd01ec47078c9bf5e2))

## [16.40.0](https://github.com/FitGlue/server/compare/v16.39.0...v16.40.0) (2026-06-09)


### Features

* **registry:** add supports_non_blocking to PluginManifest proto ([9f902ba](https://github.com/FitGlue/server/commit/9f902ba86ef1b05556909d62b9f38a115861124b))


### Bug Fixes

* **api-client:** correct Firestore aggregation type assertion in platform stats ([f9257a4](https://github.com/FitGlue/server/commit/f9257a43a10509eaf8e962f57f3624d8b23c5f82))

## [16.39.0](https://github.com/FitGlue/server/compare/v16.38.0...v16.39.0) (2026-06-09)


### Features

* **enricher:** add hDrop sweat analysis enricher and non-blocking pending input ([22c6f7c](https://github.com/FitGlue/server/commit/22c6f7c232c5869df373501b8d32740c1714d038))
* **photos:** add server-side image resize in photo-upload enricher ([835b5ae](https://github.com/FitGlue/server/commit/835b5ae0267d78210870681c8fe1565d574526f4))

## [16.38.0](https://github.com/FitGlue/server/compare/v16.37.1...v16.38.0) (2026-06-08)


### Features

* **api-client:** expose platform stats in plugin registry marketing mode ([1ea1c14](https://github.com/FitGlue/server/commit/1ea1c143eef99f3e86792c64494410dc6d8e72a8))
* **pipeline:** implement AdminListPipelineRuns gRPC method ([5cfed4d](https://github.com/FitGlue/server/commit/5cfed4dfe50fa70a4b8921bbce538bd3b288500e))

### [16.37.1](https://github.com/FitGlue/server/compare/v16.37.0...v16.37.1) (2026-06-08)


### Bug Fixes

* **notifications:** implement SetFCMToken in user service ([f2a2b78](https://github.com/FitGlue/server/commit/f2a2b78a5585be60eedf6f1c55f590391ffe5e1d))

## [16.37.0](https://github.com/FitGlue/server/compare/v16.36.1...v16.37.0) (2026-06-06)


### Features

* **fitbit:** enhance time handling by fetching user's Fitbit profile timezone ([3baaa5e](https://github.com/FitGlue/server/commit/3baaa5e97ee0fd7592031bcde4015076f8ad3c88))

### [16.36.1](https://github.com/FitGlue/server/compare/v16.36.0...v16.36.1) (2026-06-03)


### Bug Fixes

* **firestore:** improve time handling in getTime helper for Firestore data ([9a7048a](https://github.com/FitGlue/server/commit/9a7048a231ab69774b0a487d581cfae13674e8c3))

## [16.36.0](https://github.com/FitGlue/server/compare/v16.35.4...v16.36.0) (2026-06-03)


### Features

* **webhook:** fetch Strava streams for HR, GPS, speed, cadence and power data ([c7abf26](https://github.com/FitGlue/server/commit/c7abf266153ddd2c3f6c3f2cea97bf2daee38602))

### [16.35.4](https://github.com/FitGlue/server/compare/v16.35.3...v16.35.4) (2026-06-03)


### Bug Fixes

* **webhook:** acknowledge receipt immediately to prevent Strava duplicate retries ([7843eb2](https://github.com/FitGlue/server/commit/7843eb283b9060de519459dc0dea87dc3dd7ff92))

### [16.35.3](https://github.com/FitGlue/server/compare/v16.35.2...v16.35.3) (2026-06-02)


### Bug Fixes

* **notifications:** include title/body in FCM data map for SW fallback ([f200c3b](https://github.com/FitGlue/server/commit/f200c3becfd121ecf764fc77e372d24901f46457))
* **notifications:** wire SYSTEM_EMAIL and EMAIL_APP_PASSWORD to notification service ([8f43da5](https://github.com/FitGlue/server/commit/8f43da5b327183f46db78563a3fe53646fc50e2e))
* **terraform:** grant notification service account secret accessor role ([5d034b6](https://github.com/FitGlue/server/commit/5d034b63ac23de426b5ea0f42d7eb3053291ef6a))
* **webhook:** parse Strava API response into StandardizedActivity ([725bfa9](https://github.com/FitGlue/server/commit/725bfa986aae187eb0794ec68565036d2b94dbee))

### [16.35.2](https://github.com/FitGlue/server/compare/v16.35.1...v16.35.2) (2026-06-02)


### Bug Fixes

* **terraform:** add Strava and Fitbit OAuth client credentials to api-webhook ([17adbce](https://github.com/FitGlue/server/commit/17adbce6a1fa21113ab8b6d1e2dcc34be41216e5))

### [16.35.1](https://github.com/FitGlue/server/compare/v16.35.0...v16.35.1) (2026-06-02)


### Bug Fixes

* **webhook:** convert Strava athlete ID to int64 for Firestore lookup ([95c56be](https://github.com/FitGlue/server/commit/95c56be9771d1c3168ddd45d6f3ea64f46ca2856))

## [16.35.0](https://github.com/FitGlue/server/compare/v16.34.6...v16.35.0) (2026-06-02)


### Features

* **refactor:** Strava and Fitbit integrations to remove deprecated methods and improve token handling ([ec3fdf3](https://github.com/FitGlue/server/commit/ec3fdf303327cec8a169442d6c7ff38ca478bd67))


### Bug Fixes

* **webhook:** use FirestoreTokenSource for Strava and Fitbit token refresh ([ee6af1d](https://github.com/FitGlue/server/commit/ee6af1dc333e8eadfbcd16e651d60c80a069240d))

### [16.34.6](https://github.com/FitGlue/server/compare/v16.34.5...v16.34.6) (2026-06-02)


### Bug Fixes

* **enricher:** provision lag topic infra and extend Fitbit HR retry windows ([f66bb15](https://github.com/FitGlue/server/commit/f66bb15e68ccf3fce093a8fde01236a56bcd9253))
* **terraform:** use project-level IAM for Pub/Sub service agent dead-letter grants ([3665500](https://github.com/FitGlue/server/commit/3665500c9c0f526ffa5dc7d44ba6bece66b01251))

### [16.34.5](https://github.com/FitGlue/server/compare/v16.34.4...v16.34.5) (2026-06-02)


### Bug Fixes

* **enrichers:** re-apply completed pending inputs on subsequent resume passes ([9dc3dc1](https://github.com/FitGlue/server/commit/9dc3dc15c9f6866663a88cd640bb0eaeb9540881))
* **fit-file-hr:** re-apply completed FIT file data on subsequent resume passes ([7a14d32](https://github.com/FitGlue/server/commit/7a14d322d5b2a2133a4113f0437b6609a55bb380))

### [16.34.4](https://github.com/FitGlue/server/compare/v16.34.3...v16.34.4) (2026-06-02)


### Bug Fixes

* **oauth:** add legacy /auth/{provider}/callback route for Fitbit and other providers ([cdb6df9](https://github.com/FitGlue/server/commit/cdb6df9a8f237610cd64fba156b5c463bf09476c))

### [16.34.3](https://github.com/FitGlue/server/compare/v16.34.2...v16.34.3) (2026-06-02)


### Bug Fixes

* **registry:** update showcase externalUrlTemplate to /showcase/{id} ([250b2bf](https://github.com/FitGlue/server/commit/250b2bfbc4b9d6a79d758bc81a6265abe11271df))

### [16.34.2](https://github.com/FitGlue/server/compare/v16.34.1...v16.34.2) (2026-06-01)


### Bug Fixes

* **showcase:** write user_id to showcased_activities docs so they appear in settings ([ae436f7](https://github.com/FitGlue/server/commit/ae436f7d7b2f6ed58ab698e6392951a057de0b4c))

### [16.34.1](https://github.com/FitGlue/server/compare/v16.34.0...v16.34.1) (2026-06-01)

## [16.34.0](https://github.com/FitGlue/server/compare/v16.33.0...v16.34.0) (2026-06-01)


### Features

* **iam:** add 'api-webhook' to firestore_services local variable ([ee5669c](https://github.com/FitGlue/server/commit/ee5669c8c4aa0d70d7463882268eaaedb1786a66))

## [16.33.0](https://github.com/FitGlue/server/compare/v16.32.2...v16.33.0) (2026-06-01)


### Features

* **stats:** split synced-activity count from destination post count ([023c5c9](https://github.com/FitGlue/server/commit/023c5c9495c63786d99c3d72b6138c4970414a65))

### [16.32.2](https://github.com/FitGlue/server/compare/v16.32.1...v16.32.2) (2026-05-31)


### Bug Fixes

* **parkrun-fetcher:** handle cookie consent wall and add webdriver stealth ([0d93c5e](https://github.com/FitGlue/server/commit/0d93c5ebd48ef9ea91b809648d014a1c5d7c00e0))

### [16.32.1](https://github.com/FitGlue/server/compare/v16.32.0...v16.32.1) (2026-05-31)


### Bug Fixes

* **parkrun-fetcher:** bump Playwright to v1.60.0 and pin npm package ([64a4242](https://github.com/FitGlue/server/commit/64a424212042fdf852c3823cdb056910f2abe3a6))

## [16.32.0](https://github.com/FitGlue/server/compare/v16.31.0...v16.32.0) (2026-05-31)


### Features

* **billing:** automated trial expiry email notifications ([28cba90](https://github.com/FitGlue/server/commit/28cba9033a09af6ac154660fbb0900c9112ed108))
* **billing:** email notification system for billing events ([151af61](https://github.com/FitGlue/server/commit/151af61edda14d11c45c04eec1e8f7cc59d3fc06))
* **email:** templates for all notification types and access-granted ([e6346eb](https://github.com/FitGlue/server/commit/e6346ebdd6a234754456311c43cbc2573e5e4a16))
* **parkrun:** rebuild parkrun-fetcher Playwright service ([a0a4ee1](https://github.com/FitGlue/server/commit/a0a4ee1743144e9e2c2cd3d1aa8d77b6ca4c74f7))


### Bug Fixes

* **api-admin:** use snake_case is_admin field when checking admin status ([1ad7e50](https://github.com/FitGlue/server/commit/1ad7e5030199b64aecde463b3b62789fc10e8a75))
* **ci:** include src/typescript in workspace for parkrun-fetcher build ([0f24352](https://github.com/FitGlue/server/commit/0f24352a6ffd2b722a029e85ddc96e26bc03f0d9))

## [16.31.0](https://github.com/FitGlue/server/compare/v16.30.5...v16.31.0) (2026-05-31)


### Features

* **notifications:** add PIPELINE_CANCELLED notification type ([df68ca9](https://github.com/FitGlue/server/commit/df68ca9b61e7ede6888302a6491de92be7a4ae73))


### Bug Fixes

* **pipeline:** add PublishJSON to pipeline test mock publishers ([1fbd348](https://github.com/FitGlue/server/commit/1fbd348f864d52d7b0437159cd01deb6b5b6c5b5))

### [16.30.5](https://github.com/FitGlue/server/compare/v16.30.4...v16.30.5) (2026-05-31)


### Bug Fixes

* **pipeline:** prevent double name-suffix on pending-input resume ([b78be46](https://github.com/FitGlue/server/commit/b78be46bb17866d5649b962ca63ee8f2e3d37b51)), closes [#24](https://github.com/FitGlue/server/issues/24)
* **terraform:** grant notification SA correct IAM permissions ([2addaef](https://github.com/FitGlue/server/commit/2addaef1c911aa586f26e5e508568e67db0be363))
* **webhook:** update bounceback check to use standardized activity start time ([12b3eb4](https://github.com/FitGlue/server/commit/12b3eb4fbd2908143ee54b9afa654669ffa6bc06))

### [16.30.4](https://github.com/FitGlue/server/compare/v16.30.3...v16.30.4) (2026-05-31)


### Bug Fixes

* **api-public:** register and implement roundup endpoints ([1268a80](https://github.com/FitGlue/server/commit/1268a8057143bc47fcb3bb5962e1229a3fa63b31))

### [16.30.3](https://github.com/FitGlue/server/compare/v16.30.2...v16.30.3) (2026-05-31)


### Bug Fixes

* **api-client:** register and implement PUT /showcase-management/roundup-settings ([1ca635c](https://github.com/FitGlue/server/commit/1ca635c6e7b425c2741de3967fc1e9b2428a6bd4))
* **api-client:** unwrap settings body before proto-unmarshal in roundup handler ([6f97d00](https://github.com/FitGlue/server/commit/6f97d00a157a99b23c6bf6c3f1a4e371937df8b8))

### [16.30.2](https://github.com/FitGlue/server/compare/v16.30.1...v16.30.2) (2026-05-31)


### Bug Fixes

* **activity:** add GetUserNotificationData stub to mock; pass nil notifications to test helper ([4d6105f](https://github.com/FitGlue/server/commit/4d6105fdf690e7d3e85ee184ded0b5ac52f4143e))
* **activity:** add nil notifications arg to all NewService calls in tests ([98c2d42](https://github.com/FitGlue/server/commit/98c2d42e4b9e21691ef338f4891babf92c3abee4))
* **activity:** correct NewService call in showcase_extra_test ([c4f10b9](https://github.com/FitGlue/server/commit/c4f10b9b3b98f18d90c61357bc06339facd7789e))
* **roundup:** filter showcase entries in Go instead of Firestore range query ([02a2773](https://github.com/FitGlue/server/commit/02a2773364ce80dd2e305330152bba335062d562))

### [16.30.1](https://github.com/FitGlue/server/compare/v16.30.0...v16.30.1) (2026-05-31)


### Bug Fixes

* **goal-tracker:** correct month-end arithmetic in period key test ([e5b84b8](https://github.com/FitGlue/server/commit/e5b84b8e1813cd8297276804d562c4e52ec71b8e))
* **terraform:** grant run.invoker to activity SA for Pub/Sub push ([b9caf76](https://github.com/FitGlue/server/commit/b9caf765bab39732149f8cc37cee4c40377ec1ad))

## [16.30.0](https://github.com/FitGlue/server/compare/v16.29.0...v16.30.0) (2026-05-31)


### Features

* **roundup:** support period_start/period_end overrides in trigger message ([c45213b](https://github.com/FitGlue/server/commit/c45213b2f4011d1e0079359c234d96b6991675fb))

## [16.29.0](https://github.com/FitGlue/server/compare/v16.28.0...v16.29.0) (2026-05-30)


### Features

* **showcase:** add roundup pages — weekly/monthly/yearly training summaries ([fc28cf6](https://github.com/FitGlue/server/commit/fc28cf64175c09820e43b635d26fe0e8b35649a5))


### Bug Fixes

* **terraform:** escape JSON in scheduler.tf base64encode calls ([9b78f2c](https://github.com/FitGlue/server/commit/9b78f2c1f76bc000de98dcbabefce6ccb7bc4a86))

## [16.28.0](https://github.com/FitGlue/server/compare/v16.27.0...v16.28.0) (2026-05-30)


### Features

* **activities:** expose id and pipeline_run_status in list response ([b7e7032](https://github.com/FitGlue/server/commit/b7e7032d7451d20c1aca0d2752c2205b90fb0fa4))


### Bug Fixes

* **hevy:** close bounceback race condition on new workout creation ([a87fc02](https://github.com/FitGlue/server/commit/a87fc02f181c32da5826b0bcc019b6cb1243ad9b))

## [16.27.0](https://github.com/FitGlue/server/compare/v16.26.0...v16.27.0) (2026-05-30)


### Features

* **enricher:** add temperature summary enricher ([7dc4c16](https://github.com/FitGlue/server/commit/7dc4c162e63205658131ba32d8a52b43afd70112))

## [16.26.0](https://github.com/FitGlue/server/compare/v16.25.5...v16.26.0) (2026-05-29)


### Features

* **pending-input:** store activity source for display in pending input ([9f9b7a5](https://github.com/FitGlue/server/commit/9f9b7a5e591495d0424d34ab3957bd2b6558a281))

### [16.25.5](https://github.com/FitGlue/server/compare/v16.25.4...v16.25.5) (2026-05-29)


### Bug Fixes

* **ical-title:** reset SportProfileName to auto-generated on blank upload title ([dd5591b](https://github.com/FitGlue/server/commit/dd5591b879136e97445442fba23da437fc604b95))

### [16.25.4](https://github.com/FitGlue/server/compare/v16.25.3...v16.25.4) (2026-05-28)


### Bug Fixes

* **pipeline:** use protojson for backfill cloud events ([dcb94ba](https://github.com/FitGlue/server/commit/dcb94ba5b67ba003b38ef59d6c4efca2ac716175))

### [16.25.3](https://github.com/FitGlue/server/compare/v16.25.2...v16.25.3) (2026-05-28)


### Bug Fixes

* **tests:** update billing and OAuth tests for webhook signature verification and API_URL requirement ([4e7e42f](https://github.com/FitGlue/server/commit/4e7e42f3f421255a81b108a3fe596b794fa5f700))
* **tests:** update tests for removed ShowcasedActivity.UserId field and updated github.NewProvider signature ([064b42b](https://github.com/FitGlue/server/commit/064b42bd13a57a2b6c8a2fee0ac295081be35f6e))

### [16.25.2](https://github.com/FitGlue/server/compare/v16.25.1...v16.25.2) (2026-05-28)


### Bug Fixes

* **pipeline:** fix nil interface trap in AlreadySynced dedup check ([438a0ea](https://github.com/FitGlue/server/commit/438a0eaa19fe664b8479a9f51f390148ecf5970b))

### [16.25.1](https://github.com/FitGlue/server/compare/v16.25.0...v16.25.1) (2026-05-28)


### Bug Fixes

* **ical-title:** allow calendar override of auto-generated file-upload names ([253d92b](https://github.com/FitGlue/server/commit/253d92b1ac4f65d88cd7253771317cfe34948275))

## [16.25.0](https://github.com/FitGlue/server/compare/v16.24.3...v16.25.0) (2026-05-28)


### Features

* **showcase:** add created_at to ShowcaseProfileEntry proto ([2010524](https://github.com/FitGlue/server/commit/20105248e417573bff137427c62f2d58dd4c64da))


### Bug Fixes

* **api-admin:** check admin status via Firestore isAdmin field ([37d146d](https://github.com/FitGlue/server/commit/37d146d705799cd8dca139b29d66cb43255e9a1b))
* **effort-score:** add same-source dedup to avoid re-enriching same activity ([1b39664](https://github.com/FitGlue/server/commit/1b39664f832539539424029800a29fd442daf8b4))
* **goal-tracker:** use activity start time for period bucketing ([6c64441](https://github.com/FitGlue/server/commit/6c644413d78bd6b9a2d89dc5dd46ae853c44ce14))

### [16.24.3](https://github.com/FitGlue/server/compare/v16.24.2...v16.24.3) (2026-05-28)


### Bug Fixes

* **api:** read pageToken (camelCase) for connection activities pagination ([3dfbdbe](https://github.com/FitGlue/server/commit/3dfbdbe851f152c4f5351467f3fc63b5bd53f013))

### [16.24.2](https://github.com/FitGlue/server/compare/v16.24.1...v16.24.2) (2026-05-27)


### Bug Fixes

* **enricher:** ensure idempotent output for duplicate Pub/Sub activities and use actual start time for AchievedAt ([346710a](https://github.com/FitGlue/server/commit/346710a1ac4794f942cb35ed9d111c0d1215183a))
* **infra:** grant pipeline service invoker permission on user service ([94dfd1a](https://github.com/FitGlue/server/commit/94dfd1ab715ae9f240377a549326898838c85779))

### [16.24.1](https://github.com/FitGlue/server/compare/v16.24.0...v16.24.1) (2026-05-27)


### Bug Fixes

* **infra:** add USER_SERVICE_URL env var to pipeline service ([d845b52](https://github.com/FitGlue/server/commit/d845b525f69c3923c1285ceddb28d1ed759b4eef))
* **infra:** use URL pattern for USER_SERVICE_URL in pipeline service ([93e5fb9](https://github.com/FitGlue/server/commit/93e5fb9497cbe7a1cf9547a0d47d273816381308))

## [16.24.0](https://github.com/FitGlue/server/compare/v16.23.9...v16.24.0) (2026-05-27)


### Features

* **connections:** add connection-scoped historical activity import ([122e199](https://github.com/FitGlue/server/commit/122e1998d1a75613726fd048184953c66ce90f95))

### [16.23.9](https://github.com/FitGlue/server/compare/v16.23.8...v16.23.9) (2026-05-27)


### Bug Fixes

* **ical): fix file-upload title, EXDATE exclusions, and multi-calendar support; fix(hevy:** prevent update race by storing record before PUT ([7c14b9a](https://github.com/FitGlue/server/commit/7c14b9abf6894cca859af413982f505f948e881a))

### [16.23.8](https://github.com/FitGlue/server/compare/v16.23.7...v16.23.8) (2026-05-27)


### Bug Fixes

* **pipeline:** stop caching typed PR enrichments to prevent showcase contamination ([a6fca83](https://github.com/FitGlue/server/commit/a6fca83f3d77aac2b09fd9c31b15a61e03a7c9c2))

### [16.23.7](https://github.com/FitGlue/server/compare/v16.23.6...v16.23.7) (2026-05-27)


### Bug Fixes

* **pipeline:** prevent GCS path collisions causing showcase data cross-contamination ([5106af1](https://github.com/FitGlue/server/commit/5106af186ed2f354553b603d499551737aa758df))

### [16.23.6](https://github.com/FitGlue/server/compare/v16.23.5...v16.23.6) (2026-05-26)


### Bug Fixes

* **enricher:** merge PersonalRecords and MuscleHeatmap into activity enrichments ([7c8b888](https://github.com/FitGlue/server/commit/7c8b88866ed4bc7d13e68574619a361b6ed54628))

### [16.23.5](https://github.com/FitGlue/server/compare/v16.23.4...v16.23.5) (2026-05-26)


### Bug Fixes

* **enrichers:** bypass dedup cache when enrichments not stored ([c96eb19](https://github.com/FitGlue/server/commit/c96eb19c30f07a0fb8d6c7476d229b274d4a066b))

### [16.23.4](https://github.com/FitGlue/server/compare/v16.23.3...v16.23.4) (2026-05-26)


### Bug Fixes

* **muscle-heatmap:** strip equipment suffixes before exercise lookup ([5961ffc](https://github.com/FitGlue/server/commit/5961ffc826e5745225799ff04c7054a6a9a281dc))

### [16.23.3](https://github.com/FitGlue/server/compare/v16.23.2...v16.23.3) (2026-05-26)


### Bug Fixes

* **enrichers:** restore typed enrichments from dedup cache ([8301b9e](https://github.com/FitGlue/server/commit/8301b9e6ccba3a826d1bf8665f44649193184adb))

### [16.23.2](https://github.com/FitGlue/server/compare/v16.23.1...v16.23.2) (2026-05-26)


### Bug Fixes

* **showcase:** restrict medal wall weight PRs to 1RM only, exclude volume records ([e75ea15](https://github.com/FitGlue/server/commit/e75ea159497bfe2c6e8619581e2ce46372851b25))

### [16.23.1](https://github.com/FitGlue/server/compare/v16.23.0...v16.23.1) (2026-05-26)


### Bug Fixes

* **showcase:** populate calories comparisonText, add photo_urls to profile entries, improve PR sorting ([c72dbe6](https://github.com/FitGlue/server/commit/c72dbe68fa71a3fc0c84f7c68f5dddf405a97e5e))

## [16.23.0](https://github.com/FitGlue/server/compare/v16.22.0...v16.23.0) (2026-05-26)


### Features

* **hevy:** expand activity type mapping and fix other-cardio duration ([d4a4289](https://github.com/FitGlue/server/commit/d4a4289f9c5deee0d414c727205617a91b02d61f))
* **notifications:** add CONNECTION_ACTION push notifications ([71f0a05](https://github.com/FitGlue/server/commit/71f0a057fc9fdf48aee33f5f3d43506b5b2d5112))


### Bug Fixes

* **enrichers:** fix HR file enricher not applying heart rate ([150acd0](https://github.com/FitGlue/server/commit/150acd0016f294071321c54f1272369a5fa581f5))
* **hevy:** surface bounceback failures that were silently swallowed ([f04a51f](https://github.com/FitGlue/server/commit/f04a51f688264d05bb7dc1da0d64096fbaa26f4b))
* **notifications:** remove unused firestore import in api-client main ([8c0ab02](https://github.com/FitGlue/server/commit/8c0ab022c0e5b3d9ea486a29c6931cf9c946e2b5))
* **notifications:** update route coverage test for new NewAPIServer signature ([a628a01](https://github.com/FitGlue/server/commit/a628a01f2485bdc5ef4e312e9bc48b9868b2f125))

## [16.22.0](https://github.com/FitGlue/server/compare/v16.21.2...v16.22.0) (2026-05-24)


### Features

* **pipeline:** auto-resolve parkrun pending inputs via Cloud Scheduler ([09fb0dc](https://github.com/FitGlue/server/commit/09fb0dce46b364eb8145af03476d7b2716806f9e))

### [16.21.2](https://github.com/FitGlue/server/compare/v16.21.1...v16.21.2) (2026-05-24)


### Bug Fixes

* **showcase:** correct zone count to 6 and fix camelCase GCS blob field names ([34ede19](https://github.com/FitGlue/server/commit/34ede193b64f3e99aba23113b1c499a9ade2ad53))

### [16.21.1](https://github.com/FitGlue/server/compare/v16.21.0...v16.21.1) (2026-05-23)

## [16.21.0](https://github.com/FitGlue/server/compare/v16.20.0...v16.21.0) (2026-05-23)


### Features

* **showcase:** compute lifetime HR zone split from activity data ([e7c86e0](https://github.com/FitGlue/server/commit/e7c86e0a3dc8695a1e31bf6ccaa61f7db5f33482))


### Bug Fixes

* **enricher:** weather enricher retries on non-transient errors ([bbb7054](https://github.com/FitGlue/server/commit/bbb7054e2437df80c96c8ca53dba985e05c08ca6))

## [16.20.0](https://github.com/FitGlue/server/compare/v16.19.0...v16.20.0) (2026-05-22)


### Features

* **showcase:** heatmap covers full history since first activity ([a102609](https://github.com/FitGlue/server/commit/a102609b3d03cfa2a76df21a80f364a148800588))

## [16.19.0](https://github.com/FitGlue/server/compare/v16.18.0...v16.19.0) (2026-05-22)


### Features

* **showcase:** compute streak heatmap + wire route thumbnail URL through pipeline ([c953eac](https://github.com/FitGlue/server/commit/c953eac11f18d38e85aebd8e8dbdf707e6805dda))

## [16.18.0](https://github.com/FitGlue/server/compare/v16.17.1...v16.18.0) (2026-05-22)


### Features

* **enrichments:** add missing fields to pace, elevation, power, recovery ([2efb896](https://github.com/FitGlue/server/commit/2efb896b47efd023262edf05018a37ea8b5c4a43))

### [16.17.1](https://github.com/FitGlue/server/compare/v16.17.0...v16.17.1) (2026-05-22)


### Bug Fixes

* **showcase:** hydrate enrichments from GCS blob at read time ([7e13906](https://github.com/FitGlue/server/commit/7e13906fad529aead328863e497e56412ca503a7))

## [16.17.0](https://github.com/FitGlue/server/compare/v16.16.0...v16.17.0) (2026-05-22)


### Features

* **showcase:** add strength PRs and PR labels to profile and entries ([e639217](https://github.com/FitGlue/server/commit/e639217dd4c1d7c0ffe9718d506b1218eb075287))


### Bug Fixes

* **showcase:** include time-based PRs in profile medal wall ([9c12fc1](https://github.com/FitGlue/server/commit/9c12fc15cec26ea668efc6157449778f21bdce81))

## [16.16.0](https://github.com/FitGlue/server/compare/v16.15.1...v16.16.0) (2026-05-22)


### Features

* **pipeline:** add CancelPipelineRun RPC — cancel by run ID, no pending input required ([5394d2e](https://github.com/FitGlue/server/commit/5394d2e5d1f9d8be1820872533f6d92ced7d4631))

### [16.15.1](https://github.com/FitGlue/server/compare/v16.15.0...v16.15.1) (2026-05-22)


### Bug Fixes

* **pipeline:** write pending_input_id to pipeline run on PENDING transition ([86d00c5](https://github.com/FitGlue/server/commit/86d00c562cbf89ad1172b9cb5d3be983337a413f))

## [16.15.0](https://github.com/FitGlue/server/compare/v16.14.0...v16.15.0) (2026-05-21)


### Features

* **pipeline:** add cancel pipeline RPC for pending-input runs ([9d44c4f](https://github.com/FitGlue/server/commit/9d44c4f13cca3a028fdcf012fccffedb27c6e86b))

## [16.14.0](https://github.com/FitGlue/server/compare/v16.13.0...v16.14.0) (2026-05-21)


### Features

* **ical_title:** implement TZID-aware event time parsing and add corresponding test ([9fe3297](https://github.com/FitGlue/server/commit/9fe3297cab840aab8c8da374eaded502001887e4))

## [16.13.0](https://github.com/FitGlue/server/compare/v16.12.1...v16.13.0) (2026-05-21)


### Features

* **enricher:** add iCal title enricher provider for activity naming ([ae58f58](https://github.com/FitGlue/server/commit/ae58f58ea0f5e0b2810bf7bf26ea8102aba3fa01))

### [16.12.1](https://github.com/FitGlue/server/compare/v16.12.0...v16.12.1) (2026-05-21)


### Bug Fixes

* **pipeline:** update source handling for same-source detection and fallback configuration ([fb1d6f7](https://github.com/FitGlue/server/commit/fb1d6f7612cabe14ac2af5592a994eaf4ac73268))

## [16.12.0](https://github.com/FitGlue/server/compare/v16.11.0...v16.12.0) (2026-05-20)


### Features

* **api-client:** add GET /pipeline-runs/:runId/payload signed-URL endpoint ([0b68d58](https://github.com/FitGlue/server/commit/0b68d585b82b7bf09d13491faa0c0e485783ef33))
* **brutal-aurora:** proto changes, typed enrichments, min HR, stats, since/until filter ([eaa95bc](https://github.com/FitGlue/server/commit/eaa95bcbbaf10c12b2de9f1780d53c4e72f69915))
* **pipeline:** write ExecutionStep records for enricher batch in PipelineRun ([4e3da2d](https://github.com/FitGlue/server/commit/4e3da2d4564b5dfb860a30c3d14919f3c98311f5))
* **pipeline:** write full ExecutionStep trace (SOURCE→PARSE→GATE→ENRICHER→ROUTER) ([0c26076](https://github.com/FitGlue/server/commit/0c26076fb321727bd19c66bb4597a4d5f2b5a911))
* **showcase:** add typed enrichment outputs to 11 enricher providers ([8f18ce4](https://github.com/FitGlue/server/commit/8f18ce421b99482d1eb3f870df0d2d7b9af08b1f))
* **showcase:** populate ShowcaseProfileEntry aggregate fields on sync ([033969e](https://github.com/FitGlue/server/commit/033969e85e08a2450fecff121e5265f74c91b195))


### Bug Fixes

* **showcase:** persist typed ActivityEnrichments to ShowcasedActivity Firestore doc ([c0c22c1](https://github.com/FitGlue/server/commit/c0c22c1c53d12ad3fa10c646bc67eb101e288ef1))

## [16.11.0](https://github.com/FitGlue/server/compare/v16.10.0...v16.11.0) (2026-05-18)


### Features

* **billing:** centralise billing event recording at executor level ([d1071dc](https://github.com/FitGlue/server/commit/d1071dcbd53345110d5a26cda5da141d5d10cfd0))
* **pipeline:** support multiple sources per pipeline ([e1881dd](https://github.com/FitGlue/server/commit/e1881dda557769173d1b891eec056e986a526f5d))


### Bug Fixes

* **showcase:** recompute profile stats from entries; fix volume and distance ([c094a73](https://github.com/FitGlue/server/commit/c094a7338bf03c9b4c1a42c62e2bac3030ed4858))

## [16.10.0](https://github.com/FitGlue/server/compare/v16.9.1...v16.10.0) (2026-05-18)


### Features

* **parkrun:** enhance timezone handling for local time estimation and add tests for edge cases ([5311f0a](https://github.com/FitGlue/server/commit/5311f0ad0951cfbb4a0c99b0951985764d0465c9))

### [16.9.1](https://github.com/FitGlue/server/compare/v16.9.0...v16.9.1) (2026-05-18)


### Bug Fixes

* **executor:** filter uploads by target destination to prevent redundant processing ([48c98a0](https://github.com/FitGlue/server/commit/48c98a0df9f69c30a77f4bb08de65fee77823440))

## [16.9.0](https://github.com/FitGlue/server/compare/v16.8.0...v16.9.0) (2026-05-18)


### Features

* **pipeline:** generate unique PipelineExecutionId using UUID for fallback scenarios ([d36cb86](https://github.com/FitGlue/server/commit/d36cb865cda08d25c228591eeaca5bfa601b345c))


### Bug Fixes

* **destination:** same-source dedup, Firestore point read, Hevy response parsing ([4a6e120](https://github.com/FitGlue/server/commit/4a6e1203c8800a0450fb180f169a3506824e2562))
* **intervals:** replace auto-detect with explicit athlete ID field ([e653178](https://github.com/FitGlue/server/commit/e65317873e6bbabe54cb4a837f990adcc1132824))

## [16.8.0](https://github.com/FitGlue/server/compare/v16.7.3...v16.8.0) (2026-05-14)


### Features

* **intervals:** implement full source ingestion pipeline ([f4c6128](https://github.com/FitGlue/server/commit/f4c61280efe35cd0f6d9e7e885e431687a2f7e5b))
* **pending-inputs:** add source activity metadata fields to PendingInput proto ([38fbade](https://github.com/FitGlue/server/commit/38fbadedf4bbe9b05f49070ffd82b5d3004ca12b))


### Bug Fixes

* **pipeline:** prevent bounceback loops, duplicate pending inputs, and Strava double-posts ([22fae58](https://github.com/FitGlue/server/commit/22fae580ea9085127af871a76af920b23205b22c))
* **type-mapper:** return ActivityType via EnrichmentResult instead of direct mutation ([6045d90](https://github.com/FitGlue/server/commit/6045d90fb9b4e4eb7323960438637ca5756f2d6e))

### [16.7.3](https://github.com/FitGlue/server/compare/v16.7.2...v16.7.3) (2026-05-13)


### Bug Fixes

* **showcase:** backfill display_name from Firebase Auth JWT for accounts with missing profile names ([c49efc4](https://github.com/FitGlue/server/commit/c49efc477ac75e782970729b03bdd7ab9a88d95e))

### [16.7.2](https://github.com/FitGlue/server/compare/v16.7.1...v16.7.2) (2026-05-13)


### Bug Fixes

* **executor:** add Timestamp field to activity metadata for accurate event timing ([c52dd62](https://github.com/FitGlue/server/commit/c52dd62d6d3baac5de3a99ade15b5b86ddc321ef))

### [16.7.1](https://github.com/FitGlue/server/compare/v16.7.0...v16.7.1) (2026-05-13)


### Bug Fixes

* **manual_workout_entry:** handle cases with no workout data gracefully ([5f737a2](https://github.com/FitGlue/server/commit/5f737a2fafe518b173c8e583e0bd9485641707f2))
* **showcase:** normalize camelCase field names to snake_case in patch builder ([5a42a72](https://github.com/FitGlue/server/commit/5a42a725e22c4347b1601a18b5d9fb3f311b91d4))

## [16.7.0](https://github.com/FitGlue/server/compare/v16.6.0...v16.7.0) (2026-05-13)


### Features

* **showcase:** support partial profile updates via field-level patch ([c136f62](https://github.com/FitGlue/server/commit/c136f626138c19131faa97a6da14beae961a4743))


### Bug Fixes

* **activity:** add PatchShowcaseProfile to MockActivityStore ([eade517](https://github.com/FitGlue/server/commit/eade517b2e45d54c6291c1ec31bf84cd5d327fcb))
* **activity:** upgrade PatchShowcaseProfile mock to use func field pattern ([e9427ef](https://github.com/FitGlue/server/commit/e9427efcd3cd2324397d4477e5a96faf5a9b36e3))

## [16.6.0](https://github.com/FitGlue/server/compare/v16.5.0...v16.6.0) (2026-05-12)


### Features

* **pipeline:** enhance PR metadata handling and add formatPRValue function ([c22a960](https://github.com/FitGlue/server/commit/c22a960231244eaf43f8065a86a1bd3a398456e9))

## [16.5.0](https://github.com/FitGlue/server/compare/v16.4.0...v16.5.0) (2026-05-11)


### Features

* **enricher:** add manual workout entry and AI activity type enrichers ([3461acf](https://github.com/FitGlue/server/commit/3461acff9353a40029979f21f8709fac495156a7))
* **enrichers:** add manual workout entry, AI activity type, showcase links, and photo gallery proto ([7e02963](https://github.com/FitGlue/server/commit/7e02963eed0876ab3fa3b3734b5d29fbecd0e272))
* **photo-upload:** add photo upload enricher with pending input ([2c50109](https://github.com/FitGlue/server/commit/2c501096f827c5cda311df2c62cf54c653fc7eef))
* **photo-upload:** add photo_urls to showcase proto and activity photo upload endpoint ([532bb9e](https://github.com/FitGlue/server/commit/532bb9efb420bbac359bc5cde13393a6f6834244))
* **showcase,workout:** bio callouts, exercise library endpoint, and bug fixes ([3767e02](https://github.com/FitGlue/server/commit/3767e024222d60c9288092a8fc2dcae5d0e0febd))


### Bug Fixes

* **github:** resolve FIT file upload not committing to repository ([0bd16c6](https://github.com/FitGlue/server/commit/0bd16c60e61055a1bb15c8d467e774b50d3d64eb))
* **pipeline:** orchestrator calls EnrichResume only for matching provider ([146d919](https://github.com/FitGlue/server/commit/146d9191fc97ec82796151fb727140f63ae813c8))
* **showcase:** three bugs in photo upload, manual workout entry, and showcase URL ([b709998](https://github.com/FitGlue/server/commit/b709998ba36b19a89258dc8986afe524c9d6bec9))
* **splits:** correct per-split time calculation for structured workouts ([42a28e9](https://github.com/FitGlue/server/commit/42a28e92871c240cd91f1a855e8feaa8af41582f))
* update Hevy uploader to handle both object and array response formats for workout IDs ([4ba7a19](https://github.com/FitGlue/server/commit/4ba7a190682c6a9e12c158f7de1c87d17c6dcc50))

## [16.4.0](https://github.com/FitGlue/server/compare/v16.3.0...v16.4.0) (2026-04-13)


### Features

* generate StrengthSets for hybrid race laps and update grouping logic to prevent duplicate uploads to Hevy ([d41347a](https://github.com/FitGlue/server/commit/d41347a7d10dcff6999196a0a40926619b33e9b7))

## [16.3.0](https://github.com/FitGlue/server/compare/v16.2.0...v16.3.0) (2026-04-13)


### Features

* add is_private configuration option for Hevy destination uploads ([3c6a332](https://github.com/FitGlue/server/commit/3c6a3322d94686ff875770513fed1b0f312af2e4))

## [16.2.0](https://github.com/FitGlue/server/compare/v16.1.0...v16.2.0) (2026-04-11)


### Features

* inject BlobStore into UploadExecutor to resolve activity data from GCS during destination processing ([10598c6](https://github.com/FitGlue/server/commit/10598c6a69e8693e258c266942929ac8db3d6052))

## [16.1.0](https://github.com/FitGlue/server/compare/v16.0.1...v16.1.0) (2026-04-11)


### Features

* handle Hevy plain-string template ID responses during creation ([36878eb](https://github.com/FitGlue/server/commit/36878eb95a3974f6cfb250e7a7c5262989e1882a))

### [16.0.1](https://github.com/FitGlue/server/compare/v16.0.0...v16.0.1) (2026-04-11)

## [16.0.0](https://github.com/FitGlue/server/compare/v15.1.0...v16.0.0) (2026-04-08)


### ⚠ BREAKING CHANGES

* user.NewService now requires a baseURL parameter; test environment removed
* move from function-based to service-based infrastructure - everything, and I mean EVERYTHING has changed

### Features

* Add `default_destination` to ShowcaseProfile, introduce a dedicated bucket for showcase assets, and refine showcase data handling. ([ef0bc09](https://github.com/FitGlue/server/commit/ef0bc092b33cdee8e926a9a9e16ac642a2f3a58c))
* add activity payload sanitizer to handle legacy unescaped JSON fields in pipeline processing ([5dbd3d4](https://github.com/FitGlue/server/commit/5dbd3d4200d126b8a4f08b5e8ba334f702856c8b))
* add activity:write scope to Strava OAuth provider configuration ([d374669](https://github.com/FitGlue/server/commit/d3746695becbaa2e1f26dc497352b29edbf209ee))
* add destination service to storage_services IAM permissions ([73d557f](https://github.com/FitGlue/server/commit/73d557f28e2f5226685e20cc6eb0eadef54966f1))
* add duration_seconds to TimeMarker and update hybrid race tagger to include duration and distance data ([70606e3](https://github.com/FitGlue/server/commit/70606e3f7cf1079398988351a1f74a18d919eb7c))
* add repost fields to ActivityPayload and implement targeted destination filtering in orchestrator ([249cff9](https://github.com/FitGlue/server/commit/249cff99d41a158534777d7081f5e66567b2adc0))
* add Skipped status to enrichment results and improve Parkrun date matching logic ([8fafb91](https://github.com/FitGlue/server/commit/8fafb9164d52c3a20f7ab2397bf5e4a3862612e5))
* complete architecture overhaul ([f0e12b0](https://github.com/FitGlue/server/commit/f0e12b0424741e890c8075fda3f0c708d39f1082))
* **enricher:** implement cycling PRs and native distance tracking ([6ad8877](https://github.com/FitGlue/server/commit/6ad887709a7d317cefae040d0d3ee0f4d163ccee))
* implement admin handlers, billing portal, repost activity, and cost optimizations ([6290685](https://github.com/FitGlue/server/commit/6290685dbb18ae754cd981a6f9bee1f21a9ccbcd))
* implement centralized gRPC and HTTP logging middleware across all services and refactor error handling into a shared framework. ([859bce2](https://github.com/FitGlue/server/commit/859bce2e4bb6144b483f1e3690c8ae15e594531d))
* implement Hevy webhook parser and introduce TerminalError to prevent retries on validation failures ([1145409](https://github.com/FitGlue/server/commit/1145409b07d39adf982f4f2c9df13032847794f6))
* Implement showcase profile management, including settings, slug, activity entries, and public profile endpoints. ([7ff069a](https://github.com/FitGlue/server/commit/7ff069a966db25f66f2e48eb0419297427786ef8))
* introduce HybridRaceSummary model and support for telemetry-only laps in activity pipeline ([dec6dc9](https://github.com/FitGlue/server/commit/dec6dc96bcf856c4b15f4da0d5407053a1954592))
* more fixes due to re-arch ([1c60d28](https://github.com/FitGlue/server/commit/1c60d28ab95562840603704ce487fa6403ab065c))
* **showcase:** add owner profile metadata and remove tier gate from profile updates ([a5b58f5](https://github.com/FitGlue/server/commit/a5b58f55178f944110b45ed86298aea33f49ccf0))
* **showcase:** implement theme persistence and fix uploader timestamp handling ([09113e3](https://github.com/FitGlue/server/commit/09113e3e788ff65570b45ec202b7534fe9684ce5))
* skip pace summary for hybrid races and recalculate session total distance after tagging ([6158cbb](https://github.com/FitGlue/server/commit/6158cbb16cc08cbc9f25f96fac85cdd90eea0793))


### Bug Fixes

* buffer request body in oauth transport to allow replay on 401 retries ([ed77773](https://github.com/FitGlue/server/commit/ed77773a58a7680a4a7762dee3bf62bee391c0ce))
* change memory allocation ([e4fee17](https://github.com/FitGlue/server/commit/e4fee17ef8cc431a81daf5bc9b5445fec9986fed))
* change step order to do terraform before deploying ([eb8f55b](https://github.com/FitGlue/server/commit/eb8f55bc06276f7b069e6136deb64dcb7fa60648))
* changes from make preflight ([c5f7aa7](https://github.com/FitGlue/server/commit/c5f7aa702c5518fb7c53f3a11b0505799fbc8648))
* check correct source string for pipeline existence ([87842c7](https://github.com/FitGlue/server/commit/87842c7f0ea270389684d1e29ed35174fb352a72))
* circleci ([80554fc](https://github.com/FitGlue/server/commit/80554fc890652ddf3e2b3f40ab543f1872ac4402))
* correct terraform variable interpolation syntax in monitoring logs panel ([ef4f925](https://github.com/FitGlue/server/commit/ef4f925ba3a42d8c9f937276a20664db1adf50f4))
* couple remaining issues ([4514c37](https://github.com/FitGlue/server/commit/4514c377596610faca508f6f2455cefca01a5db8))
* destroy test env ([0d395e2](https://github.com/FitGlue/server/commit/0d395e238e5826a90853e04f131e257c2e82eb97))
* do not duplicate var names in tf across files ([7f728fe](https://github.com/FitGlue/server/commit/7f728feb931bd8ba4f648f6d1e057c58efd31c3a))
* env var cleanup ([aaf8bb3](https://github.com/FitGlue/server/commit/aaf8bb3dea0f56df97f9eb0c8dc75d87e8c55612))
* firebase rewrites and showcase processing ([37b96ab](https://github.com/FitGlue/server/commit/37b96abe30e9357582adc236eb481cb9b1082311))
* force HTTP/1.1 for Strava uploads to resolve multipart stream errors ([a56e5d3](https://github.com/FitGlue/server/commit/a56e5d32a70cad772cf8bd379fee223c8474f0b8))
* gcloud fix for circle ([66cd094](https://github.com/FitGlue/server/commit/66cd094227b346d50a03978158a4480ae1e88deb))
* generate and send back ingress API key when connecting public api key integrations ([28905f8](https://github.com/FitGlue/server/commit/28905f8d997494e6d05268ceaf0fd4b845ab9932))
* generate enum types ([e9a902d](https://github.com/FitGlue/server/commit/e9a902d286842ccede71b14d1dc6a5b005be8a71))
* gRPC and registry bullshit ([abfd5a1](https://github.com/FitGlue/server/commit/abfd5a1dd2f9bdd3f8a3e435d40098549df3088f))
* gRPC infra dial TLS ([85471ec](https://github.com/FitGlue/server/commit/85471ec765dbe539e85e193caf94f3e14ec5c505))
* let terraform do the fkin deploying ([a3aa988](https://github.com/FitGlue/server/commit/a3aa9883b8de914e9ae9b0c7177f08c869c863a5))
* missing tf closing brace ([b753068](https://github.com/FitGlue/server/commit/b753068da2c543399827e2c522986f3f0f33f13f))
* monitoring terraform ([bdb82bb](https://github.com/FitGlue/server/commit/bdb82bb01a65c912dc484e9854c3ddc8ec5ccd89))
* more circleci ([a3daa4e](https://github.com/FitGlue/server/commit/a3daa4e3156c3cc57d9ab4447f6c04feff016569))
* more converter firestore shenanigans ([b26a7d5](https://github.com/FitGlue/server/commit/b26a7d55cd50de893fb73d1059bba505c1ab061d))
* more docker fun ([5004800](https://github.com/FitGlue/server/commit/5004800103511c11be7cb7defb4ffe73ac4a85c0))
* more terraform shenanigans ([ce08ca5](https://github.com/FitGlue/server/commit/ce08ca5874297fe2949c52848f82da05eb911556))
* more tf shit ([86a1e7f](https://github.com/FitGlue/server/commit/86a1e7f9475addafcadbe43ee0902f3b6696ad3d))
* no more removing legacy from state ([2b82d03](https://github.com/FitGlue/server/commit/2b82d03fe9e7436e84089f56b89df14a2837a9a1))
* normalize expiration timestamps and add provider-specific user ID mapping for OAuth tokens ([870e199](https://github.com/FitGlue/server/commit/870e1993c77d2464038eac61b2ad18cc27a465bb))
* permissions and logging to sentry ([caa6386](https://github.com/FitGlue/server/commit/caa6386f95a8543cf626274989eae0d928146ad9))
* pipeline storage/read converter for enricher providers ([0da91e8](https://github.com/FitGlue/server/commit/0da91e82a07831824b96a50b880a4c3f10978f22))
* proto decoding problems ([c12ea7e](https://github.com/FitGlue/server/commit/c12ea7ea6bfbe1dd18f8cee10bd08bdb7e6fde0e))
* pubsub invoker perms ([64cd9ef](https://github.com/FitGlue/server/commit/64cd9efabb5bd0451d515ba37499619233c96d54))
* refactor and centralise showcase management ([5804e8e](https://github.com/FitGlue/server/commit/5804e8e816cba11ab37521eae139935c7f5a9ac9))
* remove service-level go mod ([f58e9b5](https://github.com/FitGlue/server/commit/f58e9b5e6fc39da88eb506e08b3f0f47cf1a3a75))
* server start panics ([292fb88](https://github.com/FitGlue/server/commit/292fb882c3ffdef94fd90f53b73993ea11ee16d7))
* showcase management saving ([13178dc](https://github.com/FitGlue/server/commit/13178dcce3501577374cb22968a0c051e5e007db))
* showcase profile ([25c440f](https://github.com/FitGlue/server/commit/25c440fd70263ab0d70327cf02518c086199f315))
* **showcase:** add owner display name fallback and refine settings logic ([b89087a](https://github.com/FitGlue/server/commit/b89087aec764b563fa5af2c788004b9b6e3e5dc9))
* split terraform ([6ab6377](https://github.com/FitGlue/server/commit/6ab6377f17892e51c7b5f56ac11867c3f50f3d19))
* terraform issue ([9928828](https://github.com/FitGlue/server/commit/99288286d6f9daf0d476ca8bc767bc216bdfdabf))
* tf error, pipeline edit/disable error ([12ba10e](https://github.com/FitGlue/server/commit/12ba10e44d1a5793ada353b23435f8f19c2960c5))
* tire field normalisation layer ([926bd09](https://github.com/FitGlue/server/commit/926bd095c4695c72c915cb95d129ef4d44801882))
* update Hevy API key header from x-api-key to api-key ([a02d9bf](https://github.com/FitGlue/server/commit/a02d9bfdba99ba56cb5d2cb0b06c91d9d239dae3))
* update monitoring dashboard logs panel to include specific columns and remove redundant resource filter ([c37e004](https://github.com/FitGlue/server/commit/c37e0042c7d48a0a7db35e6f7fb0b38df7cca130))
* various bugs with new architecture ([908bc32](https://github.com/FitGlue/server/commit/908bc32eed000caa9739ff8b757a578bb97d6d52))

## [15.1.0](https://github.com/FitGlue/server/compare/v15.0.0...v15.1.0) (2026-02-21)


### Features

* add mobile activity listing endpoint and improve activity name formatting ([b3319f1](https://github.com/FitGlue/server/commit/b3319f15e4cdc767c77c1daf566a35dcc01d6005))

## [15.0.0](https://github.com/FitGlue/server/compare/v14.14.0...v15.0.0) (2026-02-19)


### ⚠ BREAKING CHANGES

* user-data-handler /counters endpoint now returns the
array directly instead of wrapping it in { counters: [...] }.

- Add ShowcaseTheme proto message and field on ShowcaseProfile
- Implement theme validation and persistence in showcase-management-handler
- Expose theme data in showcase-handler API responses for activity and profile pages
- Add FIT file download from GCS and binary commit to GitHub in github-uploader
- Ground AI banner image prompts in real-world fitness settings with exercise-specific guidance
- Fix destination display names (TrainingPeaks, Intervals.icu, Google Sheets, GitHub) in
  generated enum formatters and Go formatters; delegate FormatDestinationName to shared formatters
- Refactor mobile-source-handler to use shared CloudEvent unwrapping from framework
- Expand cascade delete in user-profile-handler to cover all subcollections and API keys
- Fix pipeline-runs-store to only clear status_message on success, not on error
- Add Firestore converter for ShowcaseTheme

### Features

* add showcase theming, GitHub FIT uploads, and improve AI image prompts ([36bb8b2](https://github.com/FitGlue/server/commit/36bb8b22ed995b93d58fb8e916a3561a2e61ce85))

## [14.14.0](https://github.com/FitGlue/server/compare/v14.13.0...v14.14.0) (2026-02-18)


### Features

* add deferred enricher execution, same-source dedup, and per-destination enricher exclusion ([e6244c6](https://github.com/FitGlue/server/commit/e6244c617674e327668b5083af87dc107443dabf))
* add per-destination enricher exclusion and simplify event cloning ([13f514c](https://github.com/FitGlue/server/commit/13f514cbaca22babe32389c5cb0182886daaf412))

## [14.13.0](https://github.com/FitGlue/server/compare/v14.12.0...v14.13.0) (2026-02-17)


### Features

* add mobile-source-handler and Apple Health/Health Connect event sources ([6001cbd](https://github.com/FitGlue/server/commit/6001cbd9cc2e92f3dd6beb7081ef8bddc2ac2829))

## [14.12.0](https://github.com/FitGlue/server/compare/v14.11.1...v14.12.0) (2026-02-17)


### Features

* health kit/connect renaming ([1458bb2](https://github.com/FitGlue/server/commit/1458bb2459ee28d176b38a8e28fbd078bf0adf68))


### Bug Fixes

* don't log executions for mobile-sync-handler ([cfb181c](https://github.com/FitGlue/server/commit/cfb181cfd2dfeb40c88cec31910ad63c8245fd37))
* linting ([1c0abcf](https://github.com/FitGlue/server/commit/1c0abcfbfc3a958d3373adc2f68f919efdfd9fe3))
* map mobile types ([d5b7a14](https://github.com/FitGlue/server/commit/d5b7a14544e0d30590751724274e26f14f3dce3f))

### [14.11.1](https://github.com/FitGlue/server/compare/v14.11.0...v14.11.1) (2026-02-17)


### Bug Fixes

* intervals section header, recovery formatting, and pipeline status hygiene ([667f947](https://github.com/FitGlue/server/commit/667f9472620316e5fc45d5957b9d201ad3a2f053))

## [14.11.0](https://github.com/FitGlue/server/compare/v14.10.2...v14.11.0) (2026-02-17)


### Features

* Add Apple Health and Health Connect integrations with dedicated mobile sync and connect endpoints. ([5df68d2](https://github.com/FitGlue/server/commit/5df68d2a2629e59d6144fc0e27de8ccb2f1d2cd9))
* Add explicit and automatic connection for mobile health integrations and report their status. ([643770d](https://github.com/FitGlue/server/commit/643770dea4be2c7240713bce0e7f7f82567b6e49))
* Enhance `storeOAuthTokens` to accept and store additional provider-specific metadata in a single write, and enable partial updates for OAuth integration fields in Firestore converters. ([40226c9](https://github.com/FitGlue/server/commit/40226c91112af8b5cf47f459558a0287766dbd7f))


### Bug Fixes

* converters for app connections ([204ae50](https://github.com/FitGlue/server/commit/204ae50c3ae1d9eaeeef7d28dabf0e12b96a42bb))

### [14.10.2](https://github.com/FitGlue/server/compare/v14.10.1...v14.10.2) (2026-02-16)


### Bug Fixes

* email sending ([b5c5383](https://github.com/FitGlue/server/commit/b5c5383c87f7b3513fc18d19c498564e8a747d3a))

### [14.10.1](https://github.com/FitGlue/server/compare/v14.10.0...v14.10.1) (2026-02-16)


### Bug Fixes

* typescript logging ([131e60b](https://github.com/FitGlue/server/commit/131e60b9bfed7996df407092e663d8257f241c74))

## [14.10.0](https://github.com/FitGlue/server/compare/v14.9.0...v14.10.0) (2026-02-16)


### Features

* add set-level strength PRs, hevy update error tracking, and showcase profile stats ([8f78663](https://github.com/FitGlue/server/commit/8f7866318827389815037816d6e3c14755661b73))


### Bug Fixes

* hevy upload issue, more debugging for auth-email-handler ([f2423b0](https://github.com/FitGlue/server/commit/f2423b0f7e0493f71a5463fb6b8222c15a4ba97f))

## [14.9.0](https://github.com/FitGlue/server/compare/v14.8.3...v14.9.0) (2026-02-16)


### Features

* add time markers for intervals and splits, fix hevy uploader and HR zone styling ([dc9e485](https://github.com/FitGlue/server/commit/dc9e4856ea51622498232cfc3fa59f24a850cfa7))

### [14.8.3](https://github.com/FitGlue/server/compare/v14.8.2...v14.8.3) (2026-02-15)

### [14.8.2](https://github.com/FitGlue/server/compare/v14.8.1...v14.8.2) (2026-02-15)


### Bug Fixes

* use correct email ([6fe2639](https://github.com/FitGlue/server/commit/6fe2639d2aeca2fd0999bf243f264426ff8e0be1))

### [14.8.1](https://github.com/FitGlue/server/compare/v14.8.0...v14.8.1) (2026-02-15)


### Bug Fixes

* env aware urls for emails, use in app for firebase auth pages ([c9635fa](https://github.com/FitGlue/server/commit/c9635faff9614d5a7aa7fcac9ce8845638475135))
* use new routing ([91e926d](https://github.com/FitGlue/server/commit/91e926da659474dc6bf2016c7e1d0b8cf4447f3e))

## [14.8.0](https://github.com/FitGlue/server/compare/v14.7.0...v14.8.0) (2026-02-15)


### Features

* add auth-email-handler, branded email templates, and intervals enricher improvements ([b7a09f4](https://github.com/FitGlue/server/commit/b7a09f427da5a39bb96e72712e5341ffea2e30b7))


### Bug Fixes

* email export ([c289bf8](https://github.com/FitGlue/server/commit/c289bf8dbd26fb0ada0152e57ab37275cf2e5741))

## [14.7.0](https://github.com/FitGlue/server/compare/v14.6.0...v14.7.0) (2026-02-15)


### Features

* **auto-increment:** add dynamic counter selection with valueDynamicSource ([f105b50](https://github.com/FitGlue/server/commit/f105b50ecf3242a1ae7140b17b8ee530eb81fa58))


### Bug Fixes

* linting ([db0fdf2](https://github.com/FitGlue/server/commit/db0fdf260b1dcf9386c1053c404605462d6b7897))

## [14.6.0](https://github.com/FitGlue/server/compare/v14.5.3...v14.6.0) (2026-02-14)


### Features

* enricher upgrades, sliding-window PRs, ACWR recovery, hero carousel, and splits renderer ([88dca99](https://github.com/FitGlue/server/commit/88dca998dac032e805b3025fbadca3bc311cd8d8))


### Bug Fixes

* github instructions [skip ci] ([2598cb2](https://github.com/FitGlue/server/commit/2598cb24ad9820500db217163face402abc830cf))

### [14.5.3](https://github.com/FitGlue/server/compare/v14.5.2...v14.5.3) (2026-02-13)


### Bug Fixes

* showcase profile slug passdown ([ebafe7a](https://github.com/FitGlue/server/commit/ebafe7a38fb1cb8a865cbfcfb625fb54074beba2))

### [14.5.2](https://github.com/FitGlue/server/compare/v14.5.1...v14.5.2) (2026-02-13)


### Bug Fixes

* use right gcs bucket for profile pic upload ([7965fdb](https://github.com/FitGlue/server/commit/7965fdbf5f54f39a309250e995f15b52b8369ad6))

### [14.5.1](https://github.com/FitGlue/server/compare/v14.5.0...v14.5.1) (2026-02-13)


### Bug Fixes

* notification links, avatar profile photo upload CORS ([84b9243](https://github.com/FitGlue/server/commit/84b9243b14d5739a44be34abadd3933ac961fd28))

## [14.5.0](https://github.com/FitGlue/server/compare/v14.4.0...v14.5.0) (2026-02-13)


### Features

* **showcase:** owner avatar in OG/API, same-source fallback, and upload UX ([c3ea3d0](https://github.com/FitGlue/server/commit/c3ea3d00704a291c8e4556fdfdcb58aeebf77291))

## [14.4.0](https://github.com/FitGlue/server/compare/v14.3.1...v14.4.0) (2026-02-13)


### Features

* **showcase:** same-source uploader overwrite, image crop, and cache tuning ([7310018](https://github.com/FitGlue/server/commit/73100189fd2603e5a0b4b2d148f8ee6e4b47ef0d))


### Bug Fixes

* photo upload for profile ([8a9bed2](https://github.com/FitGlue/server/commit/8a9bed28110c574fef36e63fdc1fd54ab177052e))

### [14.3.1](https://github.com/FitGlue/server/compare/v14.3.0...v14.3.1) (2026-02-12)


### Bug Fixes

* missing indexes ([1120d0e](https://github.com/FitGlue/server/commit/1120d0eeb3ede11f67fba57c5f27330b48d20f16))

## [14.3.0](https://github.com/FitGlue/server/compare/v14.2.0...v14.3.0) (2026-02-12)


### Features

* **showcase:** add profile management, editable profiles, and enhanced showcase UI ([6a015c2](https://github.com/FitGlue/server/commit/6a015c2e346921aaa6b4dc6b50fa42ef85d146d6))


### Bug Fixes

* build lint etc ([139ca2d](https://github.com/FitGlue/server/commit/139ca2ddfb4bae519948ceb5d8e3c3af877ca882))
* destination fallback configs ([f7d2504](https://github.com/FitGlue/server/commit/f7d2504c42bb8c18ff8bfe7c31fe1a3e56f468d6))
* googlesheets uploader max hr ([beb28e0](https://github.com/FitGlue/server/commit/beb28e020ee13c3b6b3f1fe1c4b81549317cddac))
* linting ([a720163](https://github.com/FitGlue/server/commit/a7201631fbd3dcb2c64efdb75f2a42c48b7aa0dc))
* linting, requireAthleteTier ([011df4c](https://github.com/FitGlue/server/commit/011df4cf6b4db29f5f03bfe8fb8465f838d2fd1e))

## [14.2.0](https://github.com/FitGlue/server/compare/v14.1.1...v14.2.0) (2026-02-12)


### Features

* generate nice OG information for showcase links ([3a5d6fd](https://github.com/FitGlue/server/commit/3a5d6fd3a608ce36fb27ccdf128d242e6e21277d))


### Bug Fixes

* download pipeline run as zip ([a70a654](https://github.com/FitGlue/server/commit/a70a6544d36ad1c749a3f8bc1036903eaa885434))
* tests fail ([2a2627f](https://github.com/FitGlue/server/commit/2a2627fe8c9c096ed5154639cb059d769e887481))

### [14.1.1](https://github.com/FitGlue/server/compare/v14.1.0...v14.1.1) (2026-02-11)

## [14.1.0](https://github.com/FitGlue/server/compare/v14.0.0...v14.1.0) (2026-02-11)


### Features

* improve data exports, showcase profiles, Google Sheets max HR, and pending input messages ([9e5b843](https://github.com/FitGlue/server/commit/9e5b843a42e12855a7f2a1e869eacbe1b70d3a4c))


### Bug Fixes

* hevy handler also handles workoutId in top-level ([a06469d](https://github.com/FitGlue/server/commit/a06469dc59fb5938c9d9ad73e75676ebd48da453))
* hevy-uploader to output usable workoutId ([11c83a2](https://github.com/FitGlue/server/commit/11c83a27392ae5fe7ab550334eaf4efafbf803d6))

## [14.0.0](https://github.com/FitGlue/server/compare/v13.0.0...v14.0.0) (2026-02-11)


### ⚠ BREAKING CHANGES

* StandardizedActivity protobuf adds workout field (12).
Lap protobuf adds intensity field (6). Database interface unchanged.

- Add ENRICHER_PROVIDER_INTERVALS enum value (39) to user.proto
- Add WorkoutDefinition and WorkoutStep protobuf messages
- Add intensity field to Lap proto for interval classification
- Parse FIT Workout and WorkoutStep messages in fit_parser
- Extract lap intensity and avg heart rate from FIT Lap messages
- Preserve intensity through lap merging
- Add intervals enricher provider with smart grouping, sprint
  fade analysis, and active vs recovery comparison
- Register intervals enricher in plugin registry with configurable
  show_all_intervals, show_progression, and show_summary options
- Add enum formatter entries for Intervals (Go + TypeScript)
- Add intensity field to TCX lap parser (default empty)
- Add intervals_test.go and provider_test.go

### Features

* add intervals enricher with FIT workout/interval parsing ([25874a7](https://github.com/FitGlue/server/commit/25874a7b0ab3e157fb466341e38f0e6531ad7263))


### Bug Fixes

* add perms for getting signed url ([226bd84](https://github.com/FitGlue/server/commit/226bd84c18f0ab002c7cf486570da23c941fdc6b))
* marshaling failure for repost-handler ([ba3a1c0](https://github.com/FitGlue/server/commit/ba3a1c08308d07b7379a8a3234adf0fa8141eea9))

## [13.0.0](https://github.com/FitGlue/server/compare/v12.13.3...v13.0.0) (2026-02-11)


### ⚠ BREAKING CHANGES

* UserService constructor now requires a PluginDefaultsStore parameter.
Database interface adds GetPluginDefault, SetPluginDefault, GetShowcaseProfileByUserId,
and DeleteShowcaseProfile methods.

- Add PluginDefault protobuf message and Firestore storage layer (Go + TypeScript)
- Add plugin defaults CRUD API endpoints on user-pipelines-handler
- Auto-save source/destination configs as user-level defaults on pipeline create/update
- Add backfill script for existing users' plugin defaults
- Inject plugin defaults into repost-handler for missed destinations
- Add data-export-handler Terraform provisioning (Cloud Function, Cloud Tasks queue,
  Secret Manager email-app-password secret as env var)
- Remove @google-cloud/secret-manager direct dependency from data-export-handler
- Implement showcase profile slug healing (detect and migrate orphaned profiles
  on display name change)
- Add GetShowcaseProfileByUserId and DeleteShowcaseProfile to Database interface,
  FirestoreAdapter, and mocks
- Add /u/:slug/:id route to showcase-handler
- Update ShowcaseProfileStore mock in showcase-handler tests

### Features

* add plugin defaults, data export handler, and showcase profile slug healing ([3503965](https://github.com/FitGlue/server/commit/350396504d7f0b9957d6216ac9e2fdadb13f6536))
* **server:** add ShowcaseProfile materialization, display config DSL for pending inputs, and enricher fixes ([643ec69](https://github.com/FitGlue/server/commit/643ec6969393812d25cc7924952a1b96412566f2))

### [12.13.3](https://github.com/FitGlue/server/compare/v12.13.2...v12.13.3) (2026-02-11)


### Bug Fixes

* heart rate enricher has zone 0, google sheets proper columns ([a45ff52](https://github.com/FitGlue/server/commit/a45ff52cafcfbcfd3b2e2296b229aa833647556e))

### [12.13.2](https://github.com/FitGlue/server/compare/v12.13.1...v12.13.2) (2026-02-11)


### Bug Fixes

* token refresh when providers don't return a new one ([e096dda](https://github.com/FitGlue/server/commit/e096ddad0f5d4a99a5c7ce012ea649b25fdfc136))

### [12.13.1](https://github.com/FitGlue/server/compare/v12.13.0...v12.13.1) (2026-02-10)


### Bug Fixes

* google oauth cred retrieval ([1edc5cb](https://github.com/FitGlue/server/commit/1edc5cb28cea575602d610e6d7ada7954294bab1))
* stats for marketing site ([c54f04b](https://github.com/FitGlue/server/commit/c54f04bab0133378dd9ac96cd8710ced75d0a270))

## [12.13.0](https://github.com/FitGlue/server/compare/v12.12.0...v12.13.0) (2026-02-09)


### Features

* implement github onPipelineCreate and onPipelineDelete webhook handling ([2803cc3](https://github.com/FitGlue/server/commit/2803cc3a923c8e02adff5f97dbeaad80bee2ec73))
* **server:** add Effort Score enricher, platform stats API, lifecycle hook context, and enricher tweaks ([77c22c3](https://github.com/FitGlue/server/commit/77c22c3107a460e93d7a5a2dc07ce172828f4f1a))


### Bug Fixes

* add new integrations to converters, do not retry on prod ([c139615](https://github.com/FitGlue/server/commit/c1396154958796f5a4ce4a9bcb7281134894f15b))

## [12.12.0](https://github.com/FitGlue/server/compare/v12.11.1...v12.12.0) (2026-02-09)


### Features

* **server:** add plugin config, FIT strength parsing, TRIMP tuning, and pipeline UX improvements ([2057ab1](https://github.com/FitGlue/server/commit/2057ab187bbce0bd2bb48229453996604fbf7194))

### [12.11.1](https://github.com/FitGlue/server/compare/v12.11.0...v12.11.1) (2026-02-09)


### Bug Fixes

* oauth connections for github and google ([30684e9](https://github.com/FitGlue/server/commit/30684e99f1ce672005f5f001b40d33cb0f686af8))

## [12.11.0](https://github.com/FitGlue/server/compare/v12.10.0...v12.11.0) (2026-02-09)


### Features

* enable google connection and sheets destination ([40a37ac](https://github.com/FitGlue/server/commit/40a37accb871ba295501f2c9a0d81674f2ac7870))
* **server:** add GitHub integration, pipeline sync notifications, and generated enum parsers ([ef39e6f](https://github.com/FitGlue/server/commit/ef39e6f1cfd753eb896f9e9fcc23e6d7eeaf65f2))

## [12.10.0](https://github.com/FitGlue/server/compare/v12.9.0...v12.10.0) (2026-02-08)


### Features

* **server:** standardize description formatting, add muscle group rollup, fix streak/recovery bugs, and migrate uploader types ([7ec0a75](https://github.com/FitGlue/server/commit/7ec0a755376b165172685941da232a80fa64545e))

## [12.9.0](https://github.com/FitGlue/server/compare/v12.8.3...v12.9.0) (2026-02-08)


### Features

* Consistently record uploaded activities and increment sync counts for Hevy and Strava, use original pipeline execution ID for reposted events, and refine streak tracker output. ([7a8109c](https://github.com/FitGlue/server/commit/7a8109cd82d86d8f07c5f43de06426e992499390))
* deregister stale fcm tokens ([6e74be5](https://github.com/FitGlue/server/commit/6e74be50387e84ba1d632cb98df5c131e3068d8f))


### Bug Fixes

* activity streak for any activity generation ([f37c2b6](https://github.com/FitGlue/server/commit/f37c2b6f52993ef8dafec5ea1c31177eac98be0a))
* allow deletion of temp table ([5f93e4a](https://github.com/FitGlue/server/commit/5f93e4a86cd09c71d5e61b4ce9af8359126c0356))
* monitoring dashboards, connection actions failures ([ae50b2a](https://github.com/FitGlue/server/commit/ae50b2a8e76d4e43631e3a57ce5aa5ce0abfc598))

### [12.8.3](https://github.com/FitGlue/server/compare/v12.8.2...v12.8.3) (2026-02-07)


### Bug Fixes

* auth strategies for connections actions execution ([cb9d2ae](https://github.com/FitGlue/server/commit/cb9d2ae3b2c624fc7a0156ec526b557204bebebf))

### [12.8.2](https://github.com/FitGlue/server/compare/v12.8.1...v12.8.2) (2026-02-07)


### Bug Fixes

* compositve collection for pipeline run query for admin ([1f2d00d](https://github.com/FitGlue/server/commit/1f2d00dbf66841ba4c70e2238c36084997c0f3b7))
* grant iam for cloud task creation for function ([cdce33e](https://github.com/FitGlue/server/commit/cdce33e9e85a2c5c2a8d62e89a5550062e36614e))
* grant iam permission for cloud invoker ([2da4041](https://github.com/FitGlue/server/commit/2da4041900cae748df50dda20ed0e9b72dddfd09))
* remove unnecessary index ([4338ab1](https://github.com/FitGlue/server/commit/4338ab13528dfc75551abc2ea49bb86e86ba8de9))

### [12.8.1](https://github.com/FitGlue/server/compare/v12.8.0...v12.8.1) (2026-02-07)


### Bug Fixes

* use GOOGLE_CLOUD_PROJECT env var not GCP_PROJECT ([d267999](https://github.com/FitGlue/server/commit/d26799983ee9df5aadfb772ec3cd79a501037265))

## [12.8.0](https://github.com/FitGlue/server/compare/v12.7.0...v12.8.0) (2026-02-07)


### Features

* **proto:** add IntegrationAction message to plugin schema ([f4a415e](https://github.com/FitGlue/server/commit/f4a415e12afc74b6b7696f8a8c2b5afa2ef95824))

## [12.7.0](https://github.com/FitGlue/server/compare/v12.6.0...v12.7.0) (2026-02-06)


### Features

* **enricher:** add new booster providers ([bed1666](https://github.com/FitGlue/server/commit/bed16662280a28a01fc2d2708b01ce80ea096789))

## [12.6.0](https://github.com/FitGlue/server/compare/v12.5.0...v12.6.0) (2026-02-06)


### Features

* **enricher:** add new booster providers ([3630799](https://github.com/FitGlue/server/commit/3630799e96b678cd45a78b61aed15ad6bd3c026c))


### Bug Fixes

* personal records to write section header ([7c75124](https://github.com/FitGlue/server/commit/7c751242138a522f704b926df866c6daca85105a))

## [12.5.0](https://github.com/FitGlue/server/compare/v12.4.0...v12.5.0) (2026-02-06)


### Features

* **enricher:** add Heart Rate Zones booster ([c1cf3c1](https://github.com/FitGlue/server/commit/c1cf3c12695c691ae03f7f22fcca2ee96da7d6e5))

## [12.4.0](https://github.com/FitGlue/server/compare/v12.3.1...v12.4.0) (2026-02-06)


### Features

* implement TIER_BLOCKED ghost runs and fix environment routing ([6208693](https://github.com/FitGlue/server/commit/62086935fdcf4b8953bc88a7f223b7dcccdbd592))

### [12.3.1](https://github.com/FitGlue/server/compare/v12.3.0...v12.3.1) (2026-02-05)


### Bug Fixes

* oauth handlers to redirect correctly ([07d0ae7](https://github.com/FitGlue/server/commit/07d0ae7e7b249a8120f5aac16d87ee324d0f9e84))

## [12.3.0](https://github.com/FitGlue/server/compare/v12.2.0...v12.3.0) (2026-02-05)


### Features

* implement detailed monitoring ([170a51a](https://github.com/FitGlue/server/commit/170a51a5a3c3b43f025893c8146d502f5e24f8b4))
* send summary of new waitlist users every day ([dd04950](https://github.com/FitGlue/server/commit/dd0495092af4908d79c96a2ac69a67c0759237ce))
* show branding enricher for more people ([09f6e11](https://github.com/FitGlue/server/commit/09f6e116589db299b454421576fcab4d4d41636a))


### Bug Fixes

* lint issues ([89d8cc9](https://github.com/FitGlue/server/commit/89d8cc9999ca8227c61e52b76f99fa6931718568))
* more analytics deploy failures ([1a8daf9](https://github.com/FitGlue/server/commit/1a8daf921e69034118da934e1853d6dc81d2e1a9))
* new users start as hobbyist tier ([37759b1](https://github.com/FitGlue/server/commit/37759b11505cdd4923029e82f9044d99b4726e6a))
* reorganise plugin categories ([f9beb64](https://github.com/FitGlue/server/commit/f9beb64b19ede9bc73d7e845a2cae43a1ab4c5a8))
* shared_modules missing dep ([34c8ac0](https://github.com/FitGlue/server/commit/34c8ac0a3978d1fc39aa7dc4258db8f47eb27a3a))

## [12.2.0](https://github.com/FitGlue/server/compare/v12.1.1...v12.2.0) (2026-02-05)


### Features

* add notification preferences API and fix hook-related enrichment ([1c6bd78](https://github.com/FitGlue/server/commit/1c6bd7833884ac8517771e59498c6e0d6a14030d))


### Bug Fixes

* user endpoint not allowed to delete ([845e65d](https://github.com/FitGlue/server/commit/845e65ddb3538b429306f52c29739ce04acfcc48))

### [12.1.1](https://github.com/FitGlue/server/compare/v12.1.0...v12.1.1) (2026-02-04)


### Bug Fixes

* align fit file provided heartrate data when mismatched ([bf2241a](https://github.com/FitGlue/server/commit/bf2241a91e27087180503501b7cd98871ce7768d))
* auth-on-create to expect Gen 1 events ([ff2d782](https://github.com/FitGlue/server/commit/ff2d78206087ff2d4540119f337ab7d5337df058))

## [12.1.0](https://github.com/FitGlue/server/compare/v12.0.1...v12.1.0) (2026-02-04)


### Features

* allow configuration of ai banner subject ([1e194f9](https://github.com/FitGlue/server/commit/1e194f9e91082297f0a87639852d51cc40250b45))
* use a text LLM to generate image prompt ([12934f1](https://github.com/FitGlue/server/commit/12934f16d30527da80f50c0624b6de0527ee085a))


### Bug Fixes

* actually pass workout information to original text llm generator pre image generation ([b15356d](https://github.com/FitGlue/server/commit/b15356d53e3288d8fe653c35a7b93747090188c0))
* encourage AI generator to not do text and treat title and notes as context not verbatim ([856b69b](https://github.com/FitGlue/server/commit/856b69bfd69b85f6802aae3ae20f612faedf4ab6))

### [12.0.1](https://github.com/FitGlue/server/compare/v12.0.0...v12.0.1) (2026-02-04)


### Bug Fixes

* better ai image generation ([cea62d2](https://github.com/FitGlue/server/commit/cea62d25869db94f6d80257b780e5b75785be737))

## [12.0.0](https://github.com/FitGlue/server/compare/v11.1.0...v12.0.0) (2026-02-04)


### ⚠ BREAKING CHANGES

* Update node version to v22

### Features

* upgrade node version ([d962b5d](https://github.com/FitGlue/server/commit/d962b5d8ed5a229d1d7cc4b21a62130ff7182796))


### Bug Fixes

* destinations not being read from the new subcollection properly, create firestore index ([b6c45cd](https://github.com/FitGlue/server/commit/b6c45cd74521c41149a78628e3aba9383c51204b))
* install node where needed for circleci ([32205de](https://github.com/FitGlue/server/commit/32205dec435c1a8f11329c1759b8a61b3487aeb5))
* more firestore indexes ([d7b89ca](https://github.com/FitGlue/server/commit/d7b89ca20701b8877252dd3e851baea5c7274c24))
* sentry-cli sudo install ([8c5a6b0](https://github.com/FitGlue/server/commit/8c5a6b0a274dff1a66331cadffa38ec4fbcef93e))
* set originalPayloadUri immediately ([fdbab00](https://github.com/FitGlue/server/commit/fdbab00a04a6b97ab90f1193662e1d92f4e0fef1))

## [11.1.0](https://github.com/FitGlue/server/compare/v11.0.1...v11.1.0) (2026-02-03)


### Features

* Enhance pipeline re-run capabilities with original payload storage and stale pending input clearing, and improve user deletion robustness. ([40a222b](https://github.com/FitGlue/server/commit/40a222be91d76e9c61fbbb64a1dc0790a5edd648))


### Bug Fixes

* make all fail ([51b2374](https://github.com/FitGlue/server/commit/51b23748fed3b6c03077dd3aa03a757e1d1d28c6))

### [11.0.1](https://github.com/FitGlue/server/compare/v11.0.0...v11.0.1) (2026-02-03)


### Bug Fixes

* ensure all uploaders properly fail destinations on early errors ([6277841](https://github.com/FitGlue/server/commit/6277841c29be4f32056694f8db5582aa08a504fc))

## [11.0.0](https://github.com/FitGlue/server/compare/v10.0.3...v11.0.0) (2026-02-03)


### ⚠ BREAKING CHANGES

* InputService constructor now accepts optional
PipelineRunStore parameter. Hevy exercise name normalization is less
aggressive - existing mappings may resolve differently.

Pipeline Status:
- Mark pipeline runs as SKIPPED when pending inputs are dismissed
- Add findByActivityId() to PipelineRunStore for activity lookups
- Update InputService to set pipeline status when dismissing inputs
- Fix uploaders (Strava, TrainingPeaks, Intervals, Hevy) to properly
  update destination status on early skip conditions

Hevy Uploader:
- Reduce exercise name normalization aggressiveness (keep original casing)
- Add explicit station-to-Hevy mappings for Hyrox activities
- Map exercises to correct types (distance_duration, weight_duration)
- Add custom exercise template support for unmapped exercises

Fitbit HR Enricher:
- Fix midnight-spanning activity bug for heart rate API calls
- Add tests for activities that cross day boundaries

### Bug Fixes

* improve pipeline run status management and Hevy exercise mapping ([937b5ef](https://github.com/FitGlue/server/commit/937b5ef043c57f30a8bf9791119063f8f3fb674b))
* **webhooks:** skip delete/update events gracefully, refactor FIT file HR pending input ([0674556](https://github.com/FitGlue/server/commit/0674556fd833f07bc463c10b39dc8cd73d4743a2))

### [10.0.3](https://github.com/FitGlue/server/compare/v10.0.2...v10.0.3) (2026-02-03)


### Bug Fixes

* do not expand lap[0] when HR already covers ([cab6f43](https://github.com/FitGlue/server/commit/cab6f432d7711413a2be606bc096802db46e2385))

### [10.0.2](https://github.com/FitGlue/server/compare/v10.0.1...v10.0.2) (2026-02-03)


### Bug Fixes

* farmer's carry mapping for hevy ([e70842e](https://github.com/FitGlue/server/commit/e70842e4b3d5a064e07d3675861e5d6ae79a4dbf))

### [10.0.1](https://github.com/FitGlue/server/compare/v10.0.0...v10.0.1) (2026-02-02)


### Bug Fixes

* initialise firebase admin, make it an error if can't retrieve from GCS ([743a892](https://github.com/FitGlue/server/commit/743a892066291a5c88e93e477975973d0fe949ba))

## [10.0.0](https://github.com/FitGlue/server/compare/v9.7.1...v10.0.0) (2026-02-02)


### ⚠ BREAKING CHANGES

* enhance shared modules architecture and CI/CD integration

### Features

* auto-merge laps that should be treated as one based on fit file data ([15e69ae](https://github.com/FitGlue/server/commit/15e69ae0b9b20f51c6f663ed08611ecf355ddd0b))
* enhance shared modules architecture and CI/CD integration ([846496f](https://github.com/FitGlue/server/commit/846496f44ac6fb22651ed48e4e62d383a4ed15b6))
* **enricher:** implement GCS offloading for large activity data and update enriched event handling ([5386aca](https://github.com/FitGlue/server/commit/5386aca674def450e938565470b7f61f010fda07))
* ensure pending inputs use enricher scoped document IDs ([2a37db2](https://github.com/FitGlue/server/commit/2a37db264203253d8798d829b5fdc37666d084e1))
* **pipeline:** add handlers for listing and retrieving pipeline runs ([c1dd3eb](https://github.com/FitGlue/server/commit/c1dd3eb88534adc8afbd4c1f7c2cec7c90f4000a))
* **tests:** refactor test mocks to use framework module ([18dde3c](https://github.com/FitGlue/server/commit/18dde3c2ad6614924bf08ef05b4263b909ea9575))


### Bug Fixes

* always offload activity data to GCS ([87e1b38](https://github.com/FitGlue/server/commit/87e1b3836601ef9ad692fe5a64f4226a7ad50a57))
* merge laps with 0 duration ([b67af28](https://github.com/FitGlue/server/commit/b67af28adf7a88977421cd201d9d4fe91abbd05f))
* missing barrel exports for typescript pruning ([0bc32a5](https://github.com/FitGlue/server/commit/0bc32a52e01f1c7d1c8866a85ba7ca527b82b62b))
* typescript pruning fun ([794dd24](https://github.com/FitGlue/server/commit/794dd246bef179ab2b8919299772f0a77c4bf61a))
* update lint requirements post-changes ([77485b3](https://github.com/FitGlue/server/commit/77485b3b0292ed4599082ab85993b512f954cc79))

### [9.7.1](https://github.com/FitGlue/server/compare/v9.7.0...v9.7.1) (2026-02-02)


### Bug Fixes

* **lint:** re-add 'W15' to ERROR_RULES and update exclusions for specific components ([671199a](https://github.com/FitGlue/server/commit/671199a0f404e9197786d9a9cd6fe7111d1cb768))

## [9.7.0](https://github.com/FitGlue/server/compare/v9.6.1...v9.7.0) (2026-02-02)


### Features

* **pipeline:** enhance pipeline run lifecycle tracking and error handling ([dd39c86](https://github.com/FitGlue/server/commit/dd39c86a38198106df75f99da0d56f0fac20271f))

### [9.6.1](https://github.com/FitGlue/server/compare/v9.6.0...v9.6.1) (2026-02-02)


### Bug Fixes

* pending inputs resume ([909a26b](https://github.com/FitGlue/server/commit/909a26b62e481a3c7f5d8f56f7b7cab42c0a3efb))
* test now expects resume fields ([dd5c588](https://github.com/FitGlue/server/commit/dd5c5883f2ca29d3a0c2d21a7c47ef105d1138b0))

## [9.6.0](https://github.com/FitGlue/server/compare/v9.5.0...v9.6.0) (2026-02-02)


### Features

* Introduce PipelineRun entity and status tracking for pipeline executions. ([31695bf](https://github.com/FitGlue/server/commit/31695bfc6be8db128919afc934a4a8019ab36f61))
* **pipeline:** implement GCS payload storage and destination failure tracking ([51a3ff1](https://github.com/FitGlue/server/commit/51a3ff13b5e3dc9f650594cb9a48e6e65ec214f9))

## [9.5.0](https://github.com/FitGlue/server/compare/v9.4.0...v9.5.0) (2026-02-01)


### Features

* add running dynamics support and new enricher ([6fee6e2](https://github.com/FitGlue/server/commit/6fee6e2c40379d3bd706a37bd3e9d43e867e592e))
* **server:** implement user-data-handler and migrate to user sub-collections ([9d39b54](https://github.com/FitGlue/server/commit/9d39b54a0a2d6943dbce33b0ee5f04d2fcd1a80e))


### Bug Fixes

* add pending_inputs index ([a824ebc](https://github.com/FitGlue/server/commit/a824ebca5485d664aa9d332aacacc727bab2b630))
* update wording of multiple plugins in registry ([b0bfd07](https://github.com/FitGlue/server/commit/b0bfd079140686e08a56a8f678c135921500674b))

## [9.4.0](https://github.com/FitGlue/server/compare/v9.3.1...v9.4.0) (2026-01-26)


### Features

* allow parkrun-fetcher more time to get results, add debugging to ai-banner, add skip reasons to strava-uploader ([354c80b](https://github.com/FitGlue/server/commit/354c80b1387429ab1deab9ab878db9fd056309b0))


### Bug Fixes

* generate activity_id for pending inputs correctly ([5515f18](https://github.com/FitGlue/server/commit/5515f184f94e4f2800502a7442a5327c0d307e9d))

### [9.3.1](https://github.com/FitGlue/server/compare/v9.3.0...v9.3.1) (2026-01-26)


### Bug Fixes

* parkrun matching failure and showcase not handling updates correctly ([8071c8f](https://github.com/FitGlue/server/commit/8071c8fce1a5e5a98b05529968e01b0188a952e6))

## [9.3.0](https://github.com/FitGlue/server/compare/v9.2.0...v9.3.0) (2026-01-26)


### Features

* fix strava uploader erroring on duplicate upload, fix parkrun not attempting enrichresume or initial fetching of results ([d28a393](https://github.com/FitGlue/server/commit/d28a393370e0db8ac22ae3195bcc1e5f49aca94b))


### Bug Fixes

* add PARKRUN_FETCHER_URL to enricher func ([dd52f46](https://github.com/FitGlue/server/commit/dd52f46bdd5170f67fbc7440e41e846ce74346e6))

## [9.2.0](https://github.com/FitGlue/server/compare/v9.1.0...v9.2.0) (2026-01-26)


### Features

* new parkrun-fetcher for playwright ([12b3b87](https://github.com/FitGlue/server/commit/12b3b87ee33106ac8e526ed6bfa4bbdd1783e199))


### Bug Fixes

* actually pass pipeline_id from/to firestore ([94b02ca](https://github.com/FitGlue/server/commit/94b02ca75dd42af33b21c065c0f0ae89b3645ff8))
* attempt manual deployment of parkrun-fetcher ([f284dd5](https://github.com/FitGlue/server/commit/f284dd53362e2f0153fdef02050fb3c5d69ab0d1))
* auth for internal call from parkrun results source to parkrun fetcher ([b39bbe7](https://github.com/FitGlue/server/commit/b39bbe7dcd0b82bf9ddbacedf2c9c4c80c4bc957))
* CICD prepare for parkrun-fetcher ([b68c527](https://github.com/FitGlue/server/commit/b68c527e93c15f3f7323d66a18134908297635af))
* deletion_protection false ([da29178](https://github.com/FitGlue/server/commit/da291780e51ba3427b536648faf8f05df109cde6))
* enricher only processes one pipeline now ([c99fc6c](https://github.com/FitGlue/server/commit/c99fc6c68507ca08db07e9dbe45e95530cc7e846))
* hevy update pathway now functional, parkrun results now generates unique pipeline execution ID ([c8a39d7](https://github.com/FitGlue/server/commit/c8a39d7b0f2cf05e4052a7db00248a0eda388b4f))
* more attempting to call the fetcher successfully ([ebbe765](https://github.com/FitGlue/server/commit/ebbe765d87050b56d394f059b90d801dda63e170))
* orchestrator to get all pipelines when in resume mode ([0bfe1a0](https://github.com/FitGlue/server/commit/0bfe1a0afca5bf783c1960f729717d1ce1d425b2))
* parkrun provider to correctly set pipeline_id in resume payload ([befaa7a](https://github.com/FitGlue/server/commit/befaa7a2ee5259cb4e3fd0398c693424b2ba9336))
* terraform build failure ([680730c](https://github.com/FitGlue/server/commit/680730cc5e0dc4b81bfa21aa6231116219b88fcc))
* typescript converter update [skip ci] ([09a7d9d](https://github.com/FitGlue/server/commit/09a7d9de49c6f45d8e6bce0f0fff30aaf0e438c5))

## [9.1.0](https://github.com/FitGlue/server/compare/v9.0.0...v9.1.0) (2026-01-25)


### Features

* **core:** implement pipeline fan-out, section-based descriptions, and robust loop prevention ([a0bce15](https://github.com/FitGlue/server/commit/a0bce159eb9bfc8242e4e8f20f639d5b3ff4ed2c))


### Bug Fixes

* parkrun results to use correct topic ([e65cccc](https://github.com/FitGlue/server/commit/e65cccc3c1bffa783ddcab67a25668910aaed0c4))

## [9.0.0](https://github.com/FitGlue/server/compare/v8.0.0...v9.0.0) (2026-01-25)


### ⚠ BREAKING CHANGES

* lots of changes to function params to account for passing loggers around

### Features

* add extensive debug logging to Golang funcs ([d9fdb83](https://github.com/FitGlue/server/commit/d9fdb83d97d613ffbb42c9704d32c15f0934bb19))

## [8.0.0](https://github.com/FitGlue/server/compare/v7.1.0...v8.0.0) (2026-01-24)


### ⚠ BREAKING CHANGES

* remove Mock Publish capability. It's fucked us.

### Features

* remove Mock Publish capability. It's fucked us. ([30dff86](https://github.com/FitGlue/server/commit/30dff86a107eeca735dd5ed458809db678c2c555))


### Bug Fixes

* add new pending input fields to firestore golang converter ([806ae5b](https://github.com/FitGlue/server/commit/806ae5b5d246b19edd1f8066f3ea394207aa1370))
* allow 202 http response for parkrun ([859fbac](https://github.com/FitGlue/server/commit/859fbac0ba1ba80c53a80bc718e1ff43d8b7d9aa))
* filtering of pending inputs now sends down auto_populated: true as expected ([c900ac6](https://github.com/FitGlue/server/commit/c900ac619fb8c39ae3d0542a6fbbf44f81a1a8e3))
* make converter handle original payload in JSON format not ProtobufJSON format ([7577bfd](https://github.com/FitGlue/server/commit/7577bfd4c20f0fb08156d9f8d205caff830aa922))
* parkrun html request to use browser-like user-agent header ([fb975e0](https://github.com/FitGlue/server/commit/fb975e009062d501d0a4bd8a3db04c245ef19b3b))
* **parkrun:** publish to right topic ([7d349ad](https://github.com/FitGlue/server/commit/7d349ad756526bbcd37d4ca2cc151c243cbf43c6))

## [7.1.0](https://github.com/FitGlue/server/compare/v7.0.0...v7.1.0) (2026-01-24)


### Features

* **parkrun:** add placeholder description, rich results with PB tracking, and tests ([806b076](https://github.com/FitGlue/server/commit/806b076274bf549e05349ddf2b6c19b3679c9399))

## [7.0.0](https://github.com/FitGlue/server/compare/v6.1.0...v7.0.0) (2026-01-24)


### ⚠ BREAKING CHANGES

* Adds new ActivitySource enum values (INTERVALS, TRAININGPEAKS, GOOGLESHEETS) which may require Protobuf regeneration in downstream consumers.

- Implement isBounceback() with retry logic to handle webhook race conditions
- Add source-level loop prevention check in webhook-processor before deduplication
- Filter pending inputs by auto-deadline to allow automated resolution first
- Fix Hevy external URL template (workouts -> workout)
- Exempt destination-only sources from handler coverage linting

### Features

* add bounceback detection with exponential backoff and support for destination-only sources ([625bed1](https://github.com/FitGlue/server/commit/625bed1e2620e8ef634d5a39db72749a229c5d01))


### Bug Fixes

* add index to firestore for parkrun results etc ([0cb8fec](https://github.com/FitGlue/server/commit/0cb8fec02eb43d709eff07508968b3e74995344c))

## [6.1.0](https://github.com/FitGlue/server/compare/v6.0.0...v6.1.0) (2026-01-24)


### Features

* Implement muscle heatmap image enricher provider using SVG body diagrams to visualize muscle activation. ([f5e8456](https://github.com/FitGlue/server/commit/f5e845675d7bc5d9e5768671490d98dab4e47b0d))
* many things I'm tired ([225471f](https://github.com/FitGlue/server/commit/225471fcfa975367c967af217362045cc18b5a7c))


### Bug Fixes

* ACTUALLY UPLOAD CORRECT GO ZIPS OH MY GOD ([549a58c](https://github.com/FitGlue/server/commit/549a58ce0da8efd77e564ee76c74f5416a00c19d))
* **build:** limit build concurrency to 4 jobs to prevent CI OOM ([9f23ce9](https://github.com/FitGlue/server/commit/9f23ce9db9f1f21b8b1dec49f5f9a35bfed3733f))
* **build:** limit build/test concurrency to 4 jobs to prevent CI OOM ([c491e8e](https://github.com/FitGlue/server/commit/c491e8eed175a11b925c61fae7e01b347c7b9fa6))
* fix pipeline imports ([e087c62](https://github.com/FitGlue/server/commit/e087c620d203a802b3ed7592f130e741125a8e95))
* include subdirs in go function zip builds ([29cea1b](https://github.com/FitGlue/server/commit/29cea1b06637f63ff4d2fc240ff4ab88564d4530))

## [6.0.0](https://github.com/FitGlue/server/compare/v5.0.1...v6.0.0) (2026-01-23)


### ⚠ BREAKING CHANGES

* moves enricher providers to the enricher function, so non-enricher functions aren't needlessly redployed on amending enricher providers

### Features

* move enricher providers to enricher function ([2d116c7](https://github.com/FitGlue/server/commit/2d116c74f55b2049d9e4354c02e056b07cbae7d0))
* use cloud cdn for assets bucket exposure plus SSL cert ([15964bf](https://github.com/FitGlue/server/commit/15964bf1bf344d9643dcd0120931a6a8dee4bb15))


### Bug Fixes

* enricher ordering bug ([1ca1a4c](https://github.com/FitGlue/server/commit/1ca1a4cc0e75d5a5a7c799e8432f4e14b42c0c05))
* enricher providers only return their description additions, not whole description plus addition ([012418b](https://github.com/FitGlue/server/commit/012418bfe2b9da1ae2358bf7fb1f19eb78135e74))
* failing go tests ([16cf090](https://github.com/FitGlue/server/commit/16cf090ea02d38361f2099bc50c60090abe088ad))
* make hyde park virtual route much nicer ([1838427](https://github.com/FitGlue/server/commit/183842792572886f1852a503b2842dc34365d301))
* some enrichers still returning total description ([d0b4304](https://github.com/FitGlue/server/commit/d0b43049f3ff94dd59fac077c8d0b01b2568069f))

### [5.0.1](https://github.com/FitGlue/server/compare/v5.0.0...v5.0.1) (2026-01-23)


### Bug Fixes

* bugs since refactoring ([a49f005](https://github.com/FitGlue/server/commit/a49f0059a33781efe513eb933b153f4291e4070c))
* failing showcase-handler test ([b432692](https://github.com/FitGlue/server/commit/b43269289310fa3af0fb11520e24b94d251b1a66))
* gofmt ([5b26656](https://github.com/FitGlue/server/commit/5b26656240246becccc3bd73173bab3e8ee3688d))
* image generation enricher failures ([f557697](https://github.com/FitGlue/server/commit/f557697e42532adbfc805146cbbe78bd006d96e1))
* prevent endless redeploys, fix tool build ([3b11517](https://github.com/FitGlue/server/commit/3b11517b8c72b37a587aaf87c47b3f697e893816))
* showcase assets bucket config ([43b8893](https://github.com/FitGlue/server/commit/43b88933d8d94e291411ce24e34818e1b1df8cb2))

## [5.0.0](https://github.com/FitGlue/server/compare/v4.0.1...v5.0.0) (2026-01-23)


### ⚠ BREAKING CHANGES

* **shared:** SafeHandler signature changed from (req, res, ctx) to (req, ctx).
Handlers must now return a value or a FrameworkResponse instance instead of
directly manipulating the Express 'res' object. Direct usage of 'res.send()'
or 'res.status()' in handlers is now deprecated and discouraged.
* **shared:** Standardized secret management. The direct 'GetSecret'
capability has been removed from the shared library and Go implementations.
Secrets are now injected via environment variables or accessed through
the SecretsHelper which uses SecretManagerServiceClient.
Changes include:
- Refactored 'createCloudFunction' to handle both HTTP and CloudEvent triggers.
- Introduced 'FrameworkResponse' for declarative control over response codes and headers.
- Integrated Sentry error capture directly into the framework lifecycle.
- Updated all existing handlers (admin, activities, showcase, etc.) to the new signature.
- Implemented 'Zero-Debt Convergence' standard (0 Errors, 0 Warnings) across TypeScript.
- Added Sentry environment variable injection in Terraform.
- Updated Plugin Registry with High-Fidelity Icon support (Rule G16).

### Features

* error handling, build and lint fixes ([93f0386](https://github.com/FitGlue/server/commit/93f0386010876308b0b1c31923f7f419f1bcb41d))
* sentry integration and safe handling of errors across TS ([da337e9](https://github.com/FitGlue/server/commit/da337e933112a312df94448cf6de93597ef6fbe2))


### Bug Fixes

* circleci and linter ([a4a87a7](https://github.com/FitGlue/server/commit/a4a87a704b32270e9e2934e5cf4f40aefd1d7509))
* pipelines in legacy format breaking converters ([85bd2bc](https://github.com/FitGlue/server/commit/85bd2bc51dfab8b0acfc2fabf93268616f9ef886))
* upload sourcemaps fix ([256349f](https://github.com/FitGlue/server/commit/256349f17701d09b5d46aca59dfd9a616621a3d4))


* **shared:** unify handler signatures and standardize secret management ([d6bc891](https://github.com/FitGlue/server/commit/d6bc8910e844c7a997b2a75e54bb17e5c9a4fea2))

### [4.0.1](https://github.com/FitGlue/server/compare/v4.0.0...v4.0.1) (2026-01-22)


### Bug Fixes

* sentry setup and some bug fixing ([aac480f](https://github.com/FitGlue/server/commit/aac480f56325c5a3fdcadfd7639d9820095303d4))
* sentry setup and some bug fixing ([0ad9c76](https://github.com/FitGlue/server/commit/0ad9c7690632576a242e875a0c1daea2a32c5fd0))

## [4.0.0](https://github.com/FitGlue/server/compare/v3.0.0...v4.0.0) (2026-01-22)


### ⚠ BREAKING CHANGES

* **server:** Protobuf enum updates for EnricherProviderType and DestinationType require re-generation of clients and database migrations for existing records.

### Features

* add pipeline toggling and sentry integration ([8e0f470](https://github.com/FitGlue/server/commit/8e0f4700fba9db08ab98c6d42853e1ccde198365))
* Implement Oura integration, temporarily disable various plugins, and add new deployment and secret management scripts. ([e19593c](https://github.com/FitGlue/server/commit/e19593c6db1e79186aca3d37935f62fcf323720b))
* **server:** major integration expansion and rich asset overhaul ([0e16eba](https://github.com/FitGlue/server/commit/0e16ebabad81c4d54529d3a59bf1921ad7435018))


### Bug Fixes

* change assets bucket name to use project_id prefix ([88b9e7a](https://github.com/FitGlue/server/commit/88b9e7afa7ceabd1df59d2754ffc1f604d332953))
* define variable for sentrY_dsn ([b8890c0](https://github.com/FitGlue/server/commit/b8890c02134c298447cc439aa1f8b88c5695b196))

## [3.0.0](https://github.com/FitGlue/server/compare/v2.1.0...v3.0.0) (2026-01-21)


### ⚠ BREAKING CHANGES

* Updated Database interface to include Personal Records methods and modified Protobuf definitions for integrations.

- Added new Activity Enrichers:
  - Personal Records (Cardio/Strength tracking)
  - Training Load (TRIMP calculation)
  - Spotify Tracks integration
  - Weather (Open-Meteo)
  - Elevation Summary
  - Location Naming (Reverse Geocoding)
- Implemented new Integrations:
  - Spotify (OAuth and Auth monitoring)
  - TrainingPeaks (Uploader and OAuth)
- Updated core infrastructure:
  - Extended Database interface with Firestore persistence for PRs
  - Modified Protobuf schemas for User and Events
  - Configured Terraform for new Cloud Functions and secrets
- Improved shared TypeScript utilities and registry

### Features

* add sorting to plugins and stop secrets not being defined from failing terraform ([5aaec96](https://github.com/FitGlue/server/commit/5aaec961a900654d14a0011df73a7aad54e79e2c))
* comprehensive 2026 feature expansion and core architecture updates ([6d318e3](https://github.com/FitGlue/server/commit/6d318e308c05b723fabf844078f341c12fadadea))

## [2.1.0](https://github.com/FitGlue/server/compare/v2.0.0...v2.1.0) (2026-01-21)


### Features

* Introduce new pace, cadence, power, and speed summary enrichers, refine AI companion prompt, and update user tier naming. ([8f1e325](https://github.com/FitGlue/server/commit/8f1e325fde2c16e66acbe90ee21102cea8f32f33))

## [2.0.0](https://github.com/FitGlue/server/compare/v1.9.1...v2.0.0) (2026-01-21)


### ⚠ BREAKING CHANGES

* strava source and changes to user mappings

### Features

* strava source and changes to user mappings ([f0d2b3c](https://github.com/FitGlue/server/commit/f0d2b3ce0d1067389b89678c9fc20c2b1128565f))


### Bug Fixes

* register-strava-webhook script works ([30f6cea](https://github.com/FitGlue/server/commit/30f6ceaacc18a12fffb661077a13c9633753f52c))

### [1.9.1](https://github.com/FitGlue/server/compare/v1.9.0...v1.9.1) (2026-01-21)


### Bug Fixes

* parkrun import and integrations endpoint ([940c6cb](https://github.com/FitGlue/server/commit/940c6cb8b22fccf15fb5ea1bd811287cbdafef51))

## [1.9.0](https://github.com/FitGlue/server/compare/v1.8.0...v1.9.0) (2026-01-21)


### Features

* Add AI description and heart rate summary enrichers, and refactor Fitbit HR provider to support force/skip logic. ([5085f6d](https://github.com/FitGlue/server/commit/5085f6d8aaf9be9b7a42d77643f06c54997732c6))
* improvements to enricher registration and enum usage ([f9340a2](https://github.com/FitGlue/server/commit/f9340a2c08e0aec7e5ebff30f5d506551bad2d74))
* Introduce comprehensive user tier management fields and support 'athlete' tier as 'pro' in effective tier calculations, updating Firestore converters and admin handler. ([2d2fd3f](https://github.com/FitGlue/server/commit/2d2fd3fdd42519fcc659145442bdd1974e0cc49e))
* Introduce separate `cleanTitle` and `cleanDescription` functions with distinct truncation logic and add corresponding tests. ([6ae561f](https://github.com/FitGlue/server/commit/6ae561f6e6000070eda4f9730a7b72fd945bdc53))
* Wrap full-pipeline repost messages in a CloudEvent using an updated `createCloudEvent` function that accepts a custom type. ([b8ab5d3](https://github.com/FitGlue/server/commit/b8ab5d302baebb0c31a4ea6b13a60a3fde7ae821))


### Bug Fixes

* add registry manifest to showcase response ([f59594f](https://github.com/FitGlue/server/commit/f59594f8ec0214b55068fba58ee5aafb0f7090c6))
* added firestore admin iam ([25d8506](https://github.com/FitGlue/server/commit/25d85068bf61d450f51409e726926e8f50b9ebab))
* Standardize activity, user, and pipeline execution ID fields to snake_case in repost events to prevent Go duplicate field errors. ([e7ebdb7](https://github.com/FitGlue/server/commit/e7ebdb7549458ab9882ade254149a1bf346f96cd))
* tf failures ([41b95f2](https://github.com/FitGlue/server/commit/41b95f2b2efc4c65b619662d143b6458049a36b4))

## [1.8.0](https://github.com/FitGlue/server/compare/v1.7.0...v1.8.0) (2026-01-20)


### Features

* Introduce activity counters, optimize execution fetching with projection queries, and add external URL templates for plugins. ([d10ab6b](https://github.com/FitGlue/server/commit/d10ab6b87f67f0651b264748798f8cad160df3e2))


### Bug Fixes

* emoji linter, remove unneeded firestore indexes ([91150b2](https://github.com/FitGlue/server/commit/91150b26113c77529c4fba7081401f69033b0785))

## [1.7.0](https://github.com/FitGlue/server/compare/v1.6.0...v1.7.0) (2026-01-20)


### Features

* Add execution logging controls including service-specific disabling, output truncation, and a CLI command to clean logs by service. ([4ceea0b](https://github.com/FitGlue/server/commit/4ceea0bcfa942df3c3bd4659d5414b5d2e1dcb9e))
* add new admin API handler for user management and platform statistics. ([2a0c047](https://github.com/FitGlue/server/commit/2a0c047bb50472f8bd43e4951544ca137c3b567a))
* admin capability updates ([185a89b](https://github.com/FitGlue/server/commit/185a89ba8f2c1da916cd6b47679b4a364d24a17d))
* Enhance CloudEvent publisher with extensions, add PENDING_STRAVA_PROCESSING status, and refactor repost-handler to publish to a central router topic. ([a17b69b](https://github.com/FitGlue/server/commit/a17b69b097e04a23cc6c703cc5a017fbcbf02fa2))
* Implement activity repost logic for Go uploaders and standardize TypeScript Cloud Function build entry points. ([8562e0d](https://github.com/FitGlue/server/commit/8562e0dc40fcd51ece6384b6b8ff237bebbe2430))
* Implement email prefix fallback for showcase owner display names and disable execution logging for several handlers. ([98d8978](https://github.com/FitGlue/server/commit/98d897864dcf3d4b7402b94f06be7e7445ac2ce3))
* Implement per-handler TypeScript Cloud Function deployments by adding a new build script and updating the Makefile and Terraform configurations to use individual function ZIPs. ([d16cd3f](https://github.com/FitGlue/server/commit/d16cd3f2fa65586eae435c4481bea61e4e8e6742))
* Implement standardized HTTP error logging with response body capture for Go and TypeScript HTTP clients. ([126dcf1](https://github.com/FitGlue/server/commit/126dcf12475bd5125d2eec86659e2ceb0d023424))


### Bug Fixes

* make activities-handler return ([429861b](https://github.com/FitGlue/server/commit/429861b8148a838c8cb95c76258727c953e3bf3a))
* repost-handler cloud event format publish ([0fb4a9f](https://github.com/FitGlue/server/commit/0fb4a9f98591c18a2e8de26309fff74599a9bd79))
* repost-handler not parsing previous events correctly ([7d01a48](https://github.com/FitGlue/server/commit/7d01a483c42543e7eecc2b7e5e11694f4cf327d7))

## [1.6.0](https://github.com/FitGlue/server/compare/v1.5.0...v1.6.0) (2026-01-19)


### Features

* Add comprehensive linting checks for environment variable access, protobuf freshness, enum definitions, formatter coverage, and handler configurations, alongside new enum formatter generation. ([569bb3d](https://github.com/FitGlue/server/commit/569bb3d382861ce2847fc1fe49e0364f98d06705))
* Introduce unit tests for mock and integration handlers, configure Jest, and refine linting rules with error configurations. ([bae9af8](https://github.com/FitGlue/server/commit/bae9af8df1ca2b9a79fe7fd8f5bb6a56908d15ce))


### Bug Fixes

* linting ([9045ad9](https://github.com/FitGlue/server/commit/9045ad9c56e6de30a6859a856e088b51fd39a04f))

## [1.5.0](https://github.com/FitGlue/server/compare/v1.4.0...v1.5.0) (2026-01-19)


### Features

* change parkrun location detection logic ([871a6c0](https://github.com/FitGlue/server/commit/871a6c0ce8c81763f37d92fe05ceaf2c8ce41442))
* Introduce Hevy uploader, add a repost handler, and enhance linting for destination topic and uploader consistency. ([3c65b11](https://github.com/FitGlue/server/commit/3c65b114952e09e3481b802f0b08a7bad67f6598))
* Introduce Hevy uploader, add a repost handler, and enhance linting for destination topic and uploader consistency. ([812d4b6](https://github.com/FitGlue/server/commit/812d4b6b8c55b705e93f466cfa4fba32cab81849))
* Introduce owner display name for showcased activities, populate it via Firebase Auth, and add new Parkrun integration fields. ([d919f02](https://github.com/FitGlue/server/commit/d919f02632d5681886b77f6029da8e9037f07efc))


### Bug Fixes

* increase parkrun detection distance ([ef8d299](https://github.com/FitGlue/server/commit/ef8d299674af6d92c33895bc87538f9d1fc66881))
* parkrun locations service not unwrapping JSON correctly ([77ef6c7](https://github.com/FitGlue/server/commit/77ef6c791156f5407113f90fdc1fd53aeeb1ff47))

## [1.4.0](https://github.com/FitGlue/server/compare/v1.3.0...v1.4.0) (2026-01-18)


### Features

* improve Fitbit activity type mapping by fetching detailed activity data, enhance webhook processing with per-activity traceability, and add unit tests for activity type mapping. ([7d4a35e](https://github.com/FitGlue/server/commit/7d4a35e83d8108b135c905204f9f969067614822))
* introduce Logic Gate enricher provider for conditional pipeline halting based on activity rules. ([bd9de86](https://github.com/FitGlue/server/commit/bd9de86212ff6b1f0bdb8bfb870f7911841e49e4))
* Log virtual source executions for each processed activity to enhance tracing visibility. ([6814c86](https://github.com/FitGlue/server/commit/6814c86931ae26163724c63dabb89ef93ca39df4))
* Map "structured workout" to run activity type and add related tests. ([3b75aec](https://github.com/FitGlue/server/commit/3b75aecd049eceb01516a7d2b6c7097254530d4c))
* Replace the TypeScript file upload handler with a new Go-based FIT parser function, updating pipeline configuration and protobuf definitions. ([b752fe6](https://github.com/FitGlue/server/commit/b752fe6ce7985cfc11504760fcac4c8192d11f8c))

## [1.3.0](https://github.com/FitGlue/server/compare/v1.2.1...v1.3.0) (2026-01-18)


### Features

* Add a new showcase handler cloud function to serve public activity data and viewer redirects. ([970f350](https://github.com/FitGlue/server/commit/970f350ddffbde2eea604094855de019db49e515))
* Add ShowcaseStore for typed access to showcased activities and integrate it into the showcase handler. ([1ef538d](https://github.com/FitGlue/server/commit/1ef538d5ab330f794ac830e9a693adbe1b28847c))
* Implement a new file upload handler service for direct FIT file uploads. ([a1f84e1](https://github.com/FitGlue/server/commit/a1f84e141ee86a7f03dd4069fb37969d4d6f99a4))


### Bug Fixes

* failing tests ([bfea8d5](https://github.com/FitGlue/server/commit/bfea8d51c9866521bdff57bb3a3f87229a07e7ee))

### [1.2.1](https://github.com/FitGlue/server/compare/v1.2.0...v1.2.1) (2026-01-18)

## [1.2.0](https://github.com/FitGlue/server/compare/v1.1.1...v1.2.0) (2026-01-18)


### Features

* Enhance `mapDestinations` to accept numeric and more flexible string inputs for destinations. ([a8fb836](https://github.com/FitGlue/server/commit/a8fb836a8553ba6063db5006e548ae48e102a0ff))
* introduce `PUBLIC_ID` integration authentication type and refactor configuration handler for generic auth support. ([ee25784](https://github.com/FitGlue/server/commit/ee257843ce5eedcd973af964347fca6955e45197))

### [1.1.1](https://github.com/FitGlue/server/compare/v1.1.0...v1.1.1) (2026-01-17)


### Bug Fixes

* run create-release after deploy-prod ([a6703d1](https://github.com/FitGlue/server/commit/a6703d11a56221fad4720fd07b1c56131505083f))

## [1.1.0](https://github.com/FitGlue/server/compare/v1.0.0...v1.1.0) (2026-01-17)


### Features

* add combined version control between web and server ([ae26975](https://github.com/FitGlue/server/commit/ae26975b2ec7728bef48c64158e516684852faa8))
* expand Fitbit activity type mapping, add sync count increment for billing, and refine orchestrator pipeline handling. ([480b62b](https://github.com/FitGlue/server/commit/480b62bdd7c1dd7e9602ecf37a1ae5155e88f40b))
* introduce `showcase-uploader` function and `ShowcasedActivity` data model to enable public activity sharing. ([0a70922](https://github.com/FitGlue/server/commit/0a70922d2b88daee0a7d88f9ce8b639aa4d0eaf3))


### Bug Fixes

* allow hevy api key setup via UI ([50fc54b](https://github.com/FitGlue/server/commit/50fc54bb60d55a83d7a94622de3d91af4c9277e4))
* versioning bumping ([e9112e0](https://github.com/FitGlue/server/commit/e9112e0b89ed98dad4d56eb307c5304f2c13c960))

## 1.0.0 (2026-01-17)

This is the first proper release of FitGlue Server, consolidating all development work since project inception.

### ⚠ BREAKING CHANGES

* **auth:** implement centralized AuthorizationService and refactor handlers
* Initial setup with protobuf-based architecture

### Features

* Add Parkrun results source and destination framework, refactor Parkrun enricher and plugin system ([33243ec](https://github.com/fitglue/server/commit/33243ec))
* **auth:** implement centralized AuthorizationService and refactor handlers ([bb14ee1](https://github.com/fitglue/server/commit/bb14ee1))
* Enable fetching pipeline execution details for activities ([19f14a1](https://github.com/fitglue/server/commit/19f14a1))
* Add mobile health integrations (Apple Health, Health Connect) and billing logic ([fff7354](https://github.com/fitglue/server/commit/fff7354))
* Add transformations and use cases fields to PluginManifest proto ([6279bf9](https://github.com/fitglue/server/commit/6279bf9))
* **plugins:** add marketing metadata to plugin and integration manifests ([5097f88](https://github.com/fitglue/server/commit/5097f88))
* Add example and use case details to Volume Analytics and Muscle Heatmap enrichers ([573b7db](https://github.com/fitglue/server/commit/573b7db))
* Implement profile handler and user management APIs
* Add Strava, Fitbit, and Hevy integration handlers
* Implement orchestrator and enricher pipeline processing
* Add Firebase authentication and user profile management
* **ci:** configure OIDC authentication for GCP deployments ([7064672](https://github.com/fitglue/server/commit/7064672))
* protobuf based shared types implemented ([57083bb](https://github.com/fitglue/server/commit/57083bb))
* secrets management implemented properly ([5d0a618](https://github.com/fitglue/server/commit/5d0a618))
* one-command install and local running capability ([dadec62](https://github.com/fitglue/server/commit/dadec62))
* Initial setup with Terraform, Cloud Functions, and multi-environment support ([e48db6f](https://github.com/fitglue/server/commit/e48db6f))

### Bug Fixes

* incorrect cron defs ([e013a34](https://github.com/fitglue/server/commit/e013a34))
* add new go function to build function zips python script ([2db62de](https://github.com/fitglue/server/commit/2db62de))
* billing-handler and allowUnauthenticated calls to functions using auth strategies ([5d71918](https://github.com/fitglue/server/commit/5d71918))
* add mobile-sync-handler terraform ([8d0a499](https://github.com/fitglue/server/commit/8d0a499))
* **ci:** various CI/CD fixes for OIDC authentication and cache persistence
* protobuf generation and usage fixed across all functions ([fc74c84](https://github.com/fitglue/server/commit/fc74c84))
* all version and lint issues fixed ([1a79cf5](https://github.com/fitglue/server/commit/1a79cf5))
