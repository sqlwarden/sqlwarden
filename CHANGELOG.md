# Changelog

## [0.6.0](https://github.com/sqlwarden/sqlwarden/compare/v0.5.0...v0.6.0) (2026-06-12)


### Features

* **access:** add Casbin RBAC enforcer with bun adapter ([b199f2e](https://github.com/sqlwarden/sqlwarden/commit/b199f2e7e1d1e45eda1fbc94cf564f9a4cea06a2))
* **access:** add permissions registry ([59260bf](https://github.com/sqlwarden/sqlwarden/commit/59260bfd756683f6619c6716b5978277a895f912))
* **access:** replace casbin with custom enforcer + memory cache ([f3fad07](https://github.com/sqlwarden/sqlwarden/commit/f3fad07d1da529705df24f875ea6b10dcc7c7827))
* **account:** add profile and password settings ([618222c](https://github.com/sqlwarden/sqlwarden/commit/618222cc5e3d44fac42de74949363de950f22ced))
* add org members baseline principal ([6259101](https://github.com/sqlwarden/sqlwarden/commit/625910182051a8740e51c50e2f01467bfcd8de3e))
* add org user and team context pages ([927fecd](https://github.com/sqlwarden/sqlwarden/commit/927fecd6b3e02b618bbb5f79e7d110645aeeeec0))
* add role management pages ([f27dc2f](https://github.com/sqlwarden/sqlwarden/commit/f27dc2f2357fe250812219af68464a43e85cd946))
* add scoped route pending states ([17b838b](https://github.com/sqlwarden/sqlwarden/commit/17b838bdc73105d786b3c906d938a2b6a8d29215))
* **api:** add auth handlers (register, login, refresh, logout) ([2bb944e](https://github.com/sqlwarden/sqlwarden/commit/2bb944ecb71832c6f5aeb2096796559a198bc8dd))
* **api:** add exact workspace policy id filters ([3d70a5b](https://github.com/sqlwarden/sqlwarden/commit/3d70a5b8281a75fca3f9446025d3cc64a88fd725))
* **api:** add instance admin system with setup endpoint and org creation gate ([faffc1a](https://github.com/sqlwarden/sqlwarden/commit/faffc1a5b038dfa96d5b4126ed4ff7f9fe102899))
* **api:** add instance organization list ([7a7254a](https://github.com/sqlwarden/sqlwarden/commit/7a7254ada9ee18013ada6a1d91c7ca80ff406924))
* **api:** add org management handlers ([3fe69e5](https://github.com/sqlwarden/sqlwarden/commit/3fe69e5fea4b7d8fa5e1dc41136c8d2572fdc27a))
* **api:** add org member and team discovery queries ([6dc891e](https://github.com/sqlwarden/sqlwarden/commit/6dc891ef4f54490fb2213019cd43069e4c3ca314))
* **api:** add org policy endpoints ([6f84993](https://github.com/sqlwarden/sqlwarden/commit/6f84993bae9f134dee5dc0ba2426181f7490dfad))
* **api:** add personal spaces feature gate ([f4614e2](https://github.com/sqlwarden/sqlwarden/commit/f4614e22f9d1852f27df4c0ec278900b38e66792))
* **api:** add shared list query parsing ([05c66dc](https://github.com/sqlwarden/sqlwarden/commit/05c66dce99212134b1e659bded9c46edf399a7e8))
* **api:** add team, workspace, connection, role handlers and grant reaper ([433c510](https://github.com/sqlwarden/sqlwarden/commit/433c510cd944be27943ff29c5cd83847f7ee42ca))
* **api:** add ui-ready connection list queries ([1534cf5](https://github.com/sqlwarden/sqlwarden/commit/1534cf5106ecd301c67b83399830b0b8731128a9))
* **api:** add ui-ready workspace policy list queries ([c26a325](https://github.com/sqlwarden/sqlwarden/commit/c26a325296a066b8aabbf0c915eed4b6ad8cc23a))
* **api:** add v1 auth middleware and context helpers ([0741200](https://github.com/sqlwarden/sqlwarden/commit/07412002df4a83633cbdba4d936139ee50233c42))
* **api:** add workspace and environment discovery queries ([f9010f4](https://github.com/sqlwarden/sqlwarden/commit/f9010f4de072ec6ffc23538c75b3e76e51509147))
* **api:** add workspace-scoped role management endpoints, grant org:admin policy:modify ([1e25980](https://github.com/sqlwarden/sqlwarden/commit/1e2598075882368d702ae0b8c6dbb320abbdab94))
* **api:** additional api routes, patch endpoints, and fix testcontainer tests ([bae44b4](https://github.com/sqlwarden/sqlwarden/commit/bae44b4d251f7e5f13e8e56a1fe4450927d50075))
* **api:** align me list contracts with org routes ([c67cba8](https://github.com/sqlwarden/sqlwarden/commit/c67cba818d621d569e2a3e6cb79e0d2a4f45f175))
* **api:** complete RBAC overhaul — int64 IDs, custom enforcer, environments, policy endpoints, full test coverage ([41cc7fe](https://github.com/sqlwarden/sqlwarden/commit/41cc7fef41cd3a1ec49a773d1469dffdeed5ad3f))
* **api:** consolidate per-resource /access into single /policies endpoint ([925767c](https://github.com/sqlwarden/sqlwarden/commit/925767cfacba43722e529839ae1dce017f147fb6))
* **api:** enrich policy payloads for ui ([4c39a2b](https://github.com/sqlwarden/sqlwarden/commit/4c39a2b11d7e2b78726b60437cadda9ac07a0718))
* **api:** expose effective permissions ([89893ce](https://github.com/sqlwarden/sqlwarden/commit/89893ceca8a9553aa613b8df4365500b8ad301cf))
* **api:** freeze bootstrap contract for ui ([835b067](https://github.com/sqlwarden/sqlwarden/commit/835b067d71f6f3d100625dc89c0e08eab4a79bdf))
* **api:** implement transactions where necessary with regression tests ([a5d1e31](https://github.com/sqlwarden/sqlwarden/commit/a5d1e3129f00bbf1e105838c17bc1b92f3b94783))
* **api:** paginate account org list ([fd277e7](https://github.com/sqlwarden/sqlwarden/commit/fd277e7492e0c1cadecc7d9edab45a44c7b41dda))
* **api:** paginate org and workspace role lists ([d8d460c](https://github.com/sqlwarden/sqlwarden/commit/d8d460c167e171be3f71c6f7b3c8793a93272157))
* **api:** paginate team members and instance admins ([cf37bcc](https://github.com/sqlwarden/sqlwarden/commit/cf37bcce2c219d14403ad3c36ef679a7b720fa71))
* **api:** rename /user routes to /account for terminology consistency ([4ada84d](https://github.com/sqlwarden/sqlwarden/commit/4ada84de1556f0a7ed54871109880e69aeaf3f9e))
* **api:** resolve organization lifecycle for ui ([27f4a65](https://github.com/sqlwarden/sqlwarden/commit/27f4a6552d3706dfdfefbdd1260d76e2227ae94c))
* **api:** rewrite context and middleware_v1 for new RBAC schema ([7176e08](https://github.com/sqlwarden/sqlwarden/commit/7176e080b7830a9812d23fb980c47a29731d4d12))
* **api:** standardize error responses ([fcd4575](https://github.com/sqlwarden/sqlwarden/commit/fcd4575b8d007a4de6751f736aca0652f0e1bec3))
* **api:** support direct permission bindings on workspace/environment/connection access endpoints ([a54b808](https://github.com/sqlwarden/sqlwarden/commit/a54b80847c240f6e1dd2c0711643fbe6cf3ba836))
* **api:** support granting multiple permissions in a single access policy request ([3faf1ae](https://github.com/sqlwarden/sqlwarden/commit/3faf1ae1d6ee52912c67bf4f402f5a2e40f988ee))
* **api:** unify list contracts across org resources ([00e115b](https://github.com/sqlwarden/sqlwarden/commit/00e115be0d268134917e176f38be0e4e8cee6278))
* **api:** update auth handlers for int64 account IDs ([69c8164](https://github.com/sqlwarden/sqlwarden/commit/69c81647587257ad1bd793683e80509d6e2beaee))
* **api:** update org and member handlers for new schema ([9c80793](https://github.com/sqlwarden/sqlwarden/commit/9c80793e6f2fd609e67d6c5509bc92543764dade))
* **api:** update team handlers for int64 IDs ([c462f71](https://github.com/sqlwarden/sqlwarden/commit/c462f71e4909c6a4ce00c23f25338a0301cd012d))
* **api:** update workspace handlers for new schema, remove reaper ([9d49c3a](https://github.com/sqlwarden/sqlwarden/commit/9d49c3a2d177b0b43bc5132897b84ffef88cfe1f))
* **api:** wire access/connection/encrypt deps, remove demo routes ([6640464](https://github.com/sqlwarden/sqlwarden/commit/6640464345df18acf7acc18d4148c75c943b3485))
* **api:** wrap workspace file list responses ([bc2d9f2](https://github.com/sqlwarden/sqlwarden/commit/bc2d9f2ed5646e9553105917cd8d96c714179a9e))
* **auth:** add revocable account sessions ([6cfd1bd](https://github.com/sqlwarden/sqlwarden/commit/6cfd1bd7610dd6719c73959b16e21f13ed617249))
* **auth:** make access token ttl configurable ([51c5a84](https://github.com/sqlwarden/sqlwarden/commit/51c5a84aad265a57dfeb153d1d893627a58b2319))
* **connection:** add live session manager with TTL reaper ([b52b313](https://github.com/sqlwarden/sqlwarden/commit/b52b313193cf43f4c7a87454249b09c2cb2ecdbe))
* **database:** add account, tenant, team, workspace, connection, token, role, and grant models ([bbbb264](https://github.com/sqlwarden/sqlwarden/commit/bbbb2640b16edd92e32ad288579b8c8537220d96))
* **database:** add environments layer ([4c42e8b](https://github.com/sqlwarden/sqlwarden/commit/4c42e8b64ac1c6e240d559136911bcffc8fb65f7))
* **database:** add organizations layer, remove tenants ([aaea984](https://github.com/sqlwarden/sqlwarden/commit/aaea984fe1be7db580d8629b857a9b2fbfd04655))
* **database:** add roles and policy binding layers ([5e05f26](https://github.com/sqlwarden/sqlwarden/commit/5e05f2685c8dc9bcde45d7f394ae558e7e5f3211))
* **database:** migrate Account ID to int64 (BIGSERIAL) ([6bcf05c](https://github.com/sqlwarden/sqlwarden/commit/6bcf05c2bb70ed7f4f10dfb156620ac1e8166829))
* **database:** update teams to int64 IDs and new schema ([11e22fb](https://github.com/sqlwarden/sqlwarden/commit/11e22fb7fc1e993863af51174d1735dfa6565ed7))
* **database:** update workspaces and connections to new schema ([20f58af](https://github.com/sqlwarden/sqlwarden/commit/20f58af49650482b8d87295cb18f03d4af7ab7e4))
* **db:** add support for both sqlite and postgres ([#27](https://github.com/sqlwarden/sqlwarden/issues/27)) ([d4594d3](https://github.com/sqlwarden/sqlwarden/commit/d4594d31903246b207d55e471c93333001fc1c6f))
* **db:** change database layer to use pgx and sqlc for type-safe queries ([#23](https://github.com/sqlwarden/sqlwarden/issues/23)) ([edb6efc](https://github.com/sqlwarden/sqlwarden/commit/edb6efc16126f1d29956599d96261150a8d61910))
* **db:** migrate to pgx and sqlc for type-safe queries ([edb6efc](https://github.com/sqlwarden/sqlwarden/commit/edb6efc16126f1d29956599d96261150a8d61910))
* **docker:** add docker image support and container registry publish ([#25](https://github.com/sqlwarden/sqlwarden/issues/25)) ([077b1c1](https://github.com/sqlwarden/sqlwarden/commit/077b1c1a63cb99b359d06d9a12bbef6e5f7b2678))
* **driver:** add driver interface and registry ([095cf22](https://github.com/sqlwarden/sqlwarden/commit/095cf2261859813cd40e11823c6966deb2070208))
* **driver:** add postgres, mysql, sqlite target drivers ([bfbbf0c](https://github.com/sqlwarden/sqlwarden/commit/bfbbf0c1344eba076452d875b65cc4229d5a247e))
* **encrypt:** add AES-256-GCM encryption helpers ([85269f6](https://github.com/sqlwarden/sqlwarden/commit/85269f64127754e00530481a1c1b818cee9c288e))
* **encrypt:** add encryption key rotation for connection DSNs and files ([8ac27fc](https://github.com/sqlwarden/sqlwarden/commit/8ac27fc064258eb42c5e5c0748ef29e71e5e0531))
* **enforcer:** add comprehensive enforcer test and resource hierarchy implementation ([7a90522](https://github.com/sqlwarden/sqlwarden/commit/7a905223606c3e50320d3261675de7d522baa87d))
* **environments:** add workspace environment management ([1098814](https://github.com/sqlwarden/sqlwarden/commit/10988145450c775c78652c18c8531e8f6e58b8bb))
* **files:** add file storage service foundation ([96dd545](https://github.com/sqlwarden/sqlwarden/commit/96dd5458acdf886322e204418190aecbfd63c71e))
* **files:** add IDE file browser APIs ([6ed5b64](https://github.com/sqlwarden/sqlwarden/commit/6ed5b643820926df62e4f1142bf26550d1e38d71))
* **files:** add revision retention reaper ([49a690c](https://github.com/sqlwarden/sqlwarden/commit/49a690cb2c93a45852873efb3c9e744f455b6ad7))
* **files:** add workspace file storage foundation ([1465d77](https://github.com/sqlwarden/sqlwarden/commit/1465d7797d2af870162e06311b6c1cf6155a4e5d))
* **files:** track storage backend for content ([1bac8aa](https://github.com/sqlwarden/sqlwarden/commit/1bac8aa4fa19298a9b338eb4f1c1a1c84af8fdde))
* **frontend:** add minimal home landing page ([237404b](https://github.com/sqlwarden/sqlwarden/commit/237404b77d3383f1631e9eaecbb4f7a96875ec2c))
* **frontend:** add organization app shell ([7e5724b](https://github.com/sqlwarden/sqlwarden/commit/7e5724b2043fe1ae02f9e4283a6214e462afefc4))
* **frontend:** add organization selection experience ([03378f1](https://github.com/sqlwarden/sqlwarden/commit/03378f155cf76194518055d9aaafbfbc846d2fe4))
* **frontend:** add query and api groundwork ([0f930fd](https://github.com/sqlwarden/sqlwarden/commit/0f930fd451be8b97ad1ff97915a6e8cc1679232f))
* **frontend:** add settings sidebar and migrate icons to hugeicons ([5c43a4a](https://github.com/sqlwarden/sqlwarden/commit/5c43a4a26bd2c47af6654443fbf4e94b8ca496ef))
* **frontend:** add workspace navigation shell ([d1d458b](https://github.com/sqlwarden/sqlwarden/commit/d1d458bf212d73c06e653341f1174c7ca2cc5a57))
* gate workspace pages by effective permissions ([57a5489](https://github.com/sqlwarden/sqlwarden/commit/57a548989dcd542c7c97ef2f2ec8197b2082adbe))
* **ide:** add actions slot to SidebarPane header ([551c349](https://github.com/sqlwarden/sqlwarden/commit/551c349240ac5238f4d520e07c92dcc4839cc85f))
* **ide:** add codemirror 6 sql editor with custom css-var theme ([b971aa6](https://github.com/sqlwarden/sqlwarden/commit/b971aa6c8aa593513153e7e1f0e10141fafbdd67))
* **ide:** add collapsible database panel with environment/connection tree ([f04f840](https://github.com/sqlwarden/sqlwarden/commit/f04f840f6c8046c39fb57550ac622e952d90fd15))
* **ide:** add context menu, header buttons, and create dialog to FilesPanel ([6c2b9e1](https://github.com/sqlwarden/sqlwarden/commit/6c2b9e18599eaff4d230bcaffe81cd50ae9be515))
* **ide:** add CreateItemDialog for new file/folder ([60a2d92](https://github.com/sqlwarden/sqlwarden/commit/60a2d92f1a261e80a1f3b4d2d2111edff021c5e7))
* **ide:** add delete file/folder with confirmation dialog to context menu ([252a6ce](https://github.com/sqlwarden/sqlwarden/commit/252a6cedd20110581028ace57b391be663552dfb))
* **ide:** add etag/isDirty/fileId to EditorTab, add updateTabEtag action ([7de29a4](https://github.com/sqlwarden/sqlwarden/commit/7de29a4bd1855d435c5732df0455859151775d3a))
* **ide:** add foreground query cancellation ([21d6ac9](https://github.com/sqlwarden/sqlwarden/commit/21d6ac9ff6c8f79cc541c34f8937d565dce20754))
* **ide:** add ide tab bar with type icons and close button ([a0c9e1f](https://github.com/sqlwarden/sqlwarden/commit/a0c9e1fd521fca4592959f381d98430ef3f3043b))
* **ide:** add ide toolbar with connection selector and run button ([e358902](https://github.com/sqlwarden/sqlwarden/commit/e358902832b19231f63f159c609b542f612208cf))
* **ide:** add lazy-loading collapsible file tree panel ([cba9585](https://github.com/sqlwarden/sqlwarden/commit/cba9585def18ec371d73b90391ac4dfa61fdb9aa))
* **ide:** add private file content API functions ([2a903d9](https://github.com/sqlwarden/sqlwarden/commit/2a903d96a8fb9a03f3a46e587cf2b3a83c1f86e1))
* **ide:** add reusable SidebarPane wrapper ([12c2f4d](https://github.com/sqlwarden/sqlwarden/commit/12c2f4d80b9a1d8c60cd1bb98ed5e2aca75fae06))
* **ide:** add right-click FileContextMenu with cursor-position anchor ([c5d36e0](https://github.com/sqlwarden/sqlwarden/commit/c5d36e0f3ab26418de2409aab33099a9c415358c))
* **ide:** add save button and dirty indicator to tab bar ([2211cb6](https://github.com/sqlwarden/sqlwarden/commit/2211cb6d2ec5af9720c993a7c9026ec5da6bb31f))
* **ide:** add SaveAsDialog with name input and location picker ([15f5141](https://github.com/sqlwarden/sqlwarden/commit/15f51413844294647081b4b2ccfa1beb5538eb48))
* **ide:** add tabbed results area with Results/History/Explain panes ([84f37ed](https://github.com/sqlwarden/sqlwarden/commit/84f37edb22fbec562bd8ab076b58a7be973a6902))
* **ide:** add useFileContent hook for loading file tab content ([f2ffb01](https://github.com/sqlwarden/sqlwarden/commit/f2ffb01a8a3ae70eaebfae8e76b3cefeb9316a73))
* **ide:** add YDocRegistry with BroadcastChannel provider ([d06b084](https://github.com/sqlwarden/sqlwarden/commit/d06b084cadca9a04870d7e8843a2bbe738d4ac53))
* **ide:** add zustand ide store with indexeddb persistence ([af7cb23](https://github.com/sqlwarden/sqlwarden/commit/af7cb238b41823dfd24b2058bc18871aa756c39c))
* **ide:** allow closing all tabs to reach empty state with onboarding UI ([63fe7ee](https://github.com/sqlwarden/sqlwarden/commit/63fe7ee7618d54c9e6c86769940e403c1b405bc7))
* **ide:** assemble full ide layout with store, editor, sidebar, results ([87a8d2f](https://github.com/sqlwarden/sqlwarden/commit/87a8d2f34b7d75b39ef9fbec176e79a21f91f823))
* **ide:** card-style empty state with new console, new file, open file actions ([c4b86e8](https://github.com/sqlwarden/sqlwarden/commit/c4b86e8bce14382cd5349ae7526c736ab419d1cf))
* **ide:** connect/disconnect with real sessions, backend session sync on mount ([af9a30d](https://github.com/sqlwarden/sqlwarden/commit/af9a30d99695a072ec849dc41e65a491afaba554))
* **ide:** driver icons in tabs/toolbar, green dot, connection context menu, fix select widths ([9288274](https://github.com/sqlwarden/sqlwarden/commit/9288274d7166491f75f4a1bf7b746b757c9cc342))
* **ide:** editor status bar, tab/editor border, searchable connection selector, driver icon on console tabs ([0fcb17f](https://github.com/sqlwarden/sqlwarden/commit/0fcb17f04cc2d8edffa41d4cf6cca98a63c811ea))
* **ide:** explorer toggle in title bar, rename sidebar to Explorer, consistent accent color ([2f745da](https://github.com/sqlwarden/sqlwarden/commit/2f745dac419ae7b36b3069512056fceb3c6745b0))
* **ide:** file type selector, save to toolbar, close warning, fix dirty state on open ([df67608](https://github.com/sqlwarden/sqlwarden/commit/df67608b6767dbc5531764a3c9fa345f166d3360))
* **ide:** install codemirror, zustand, idb-keyval, geist-mono ([8d10784](https://github.com/sqlwarden/sqlwarden/commit/8d107843dd6dd9fbbda6da62413451ce3bffbf53))
* **ide:** move IDE to standalone /ide/$org_slug route with top-bar user menu ([8bc058a](https://github.com/sqlwarden/sqlwarden/commit/8bc058a43b3bd830fc74b71c9608b2713ef4f3a2))
* **ide:** new connection dialog with postgres/mysql driver registry ([42f9946](https://github.com/sqlwarden/sqlwarden/commit/42f99468ea9686f20e320b5eb4296bb7da51d69c))
* **ide:** new environment dialog in explorer, retain hover on file context menu ([a561db5](https://github.com/sqlwarden/sqlwarden/commit/a561db50eaf2f065863092ca2f9e51e3412b3bf2))
* **ide:** numbered consoles with Y.js sync, cross-window tab list, fix empty state on load ([9803446](https://github.com/sqlwarden/sqlwarden/commit/98034465e7a7a71a8b83becc1b62cfc8f7b39f29))
* **ide:** per-environment connection create button with env-scoped permission check ([5cfde9d](https://github.com/sqlwarden/sqlwarden/commit/5cfde9dc8559ce2e271cf357bc51130d340e0998))
* **ide:** per-mode editor theme selection with lazy-loaded CM6 theme packs ([ec96f0c](https://github.com/sqlwarden/sqlwarden/commit/ec96f0c7b121e2740c45783b43c7637db6cb9a01))
* **ide:** per-tab query running state, spinner on tab, warn on close ([a159e04](https://github.com/sqlwarden/sqlwarden/commit/a159e04508b4b9201c750f6770a0f597df77efbe))
* **ide:** read Y.Doc content in IdeToolbar for save operations ([dd230c7](https://github.com/sqlwarden/sqlwarden/commit/dd230c7bf9fdf511f06240a6a578f7664dae21fa))
* **ide:** redesign connection dialog with driver cards, per-driver forms, driver badges in explorer ([2e97bca](https://github.com/sqlwarden/sqlwarden/commit/2e97bca7593909797491757fa40fa81aab8a9700))
* **ide:** remove kbd hints from toolbar, sync session green dot across windows ([ff4a240](https://github.com/sqlwarden/sqlwarden/commit/ff4a240e93d3a14791d414861b50263c7f61c619))
* **ide:** resizable results table with sticky row numbers, cell detail panel, keyboard nav, drag select ([ce21e68](https://github.com/sqlwarden/sqlwarden/commit/ce21e68b0c2980a5f6966f479cea2603d9c69e97))
* **ide:** roboto mono font, sidebar trigger to footer, logo in collapsed sidebar, remove IDE logo ([64f67eb](https://github.com/sqlwarden/sqlwarden/commit/64f67eb9ff2aeb093945f5c88e7b300456a2815a))
* **ide:** run query under cursor instead of full tab content on shortcut ([e2079ad](https://github.com/sqlwarden/sqlwarden/commit/e2079ad977e2596f3968b3766160f22bcad21a76))
* **ide:** run selected SQL only via EditorView registry, fall back to full document ([490af2e](https://github.com/sqlwarden/sqlwarden/commit/490af2e54d0973d5d0034b066f388a1ccc4ee2d3))
* **ide:** show Save button for connection tabs, allow saving console content as file ([539f4cb](https://github.com/sqlwarden/sqlwarden/commit/539f4cb7c09401fbf3f771496d0041e257a261d0))
* **ide:** use real driver SVG icons, replace card grid with select for driver picker ([19e0ee3](https://github.com/sqlwarden/sqlwarden/commit/19e0ee3d00828063659bf7ca452062dd867c6146))
* **ide:** warn before closing a console with unsaved content ([510f018](https://github.com/sqlwarden/sqlwarden/commit/510f018c4787e038988bb3297ac6083e7f14116b))
* **ide:** wire file content loading, cmd+s save, and loading/error states ([c3927c1](https://github.com/sqlwarden/sqlwarden/commit/c3927c19776614b18b95149b3b15f652d313bd02))
* **ide:** wire Run button to query API, display results table in results pane ([a2ae87e](https://github.com/sqlwarden/sqlwarden/commit/a2ae87e73493b8ff78e9df871e9765913112347e))
* **ide:** wire Y.Doc through SqlEditor, useFileContent, and WorkspaceIde ([6b8b549](https://github.com/sqlwarden/sqlwarden/commit/6b8b5495c8e79bdc028a9e9503335c7ba7c75d70))
* **ide:** Y.js cross-window sync with BC protocol and etag/dirty state sync ([ae38fe3](https://github.com/sqlwarden/sqlwarden/commit/ae38fe3d45ad8f1004ac23a5f3647b2c7f41a5fb))
* implement role management for organization level roles and workspace level roles ([6c299d8](https://github.com/sqlwarden/sqlwarden/commit/6c299d82517762beaeae8d187ba66b7b7a2cd475))
* initial commit - project bootstrapped from autostrada.dev ([5cdddc9](https://github.com/sqlwarden/sqlwarden/commit/5cdddc9c52eb89a249704e1ac401b111acaec4e6))
* **instance:** add editable instance settings ([15e2c62](https://github.com/sqlwarden/sqlwarden/commit/15e2c629d088831faf52bbac419e6e57c37ae20d))
* **me:** implement /me personal spaces routes, handlers, middleware and tests ([829625c](https://github.com/sqlwarden/sqlwarden/commit/829625c633ad74b5a90fd63fc822b1c1b79d9616))
* **migrations:** add postgres RBAC schema overhaul migration ([f007855](https://github.com/sqlwarden/sqlwarden/commit/f007855ea6c6fde516c060b9cb0c76beac2cccb0))
* **migrations:** add schema migrations 000003-000007 (auth, teams, casbin, connections, roles) ([5db0a6e](https://github.com/sqlwarden/sqlwarden/commit/5db0a6e51116ae69d1b2c493074426e6f7669e45))
* **migrations:** add sqlite RBAC schema overhaul migration ([aeeebcd](https://github.com/sqlwarden/sqlwarden/commit/aeeebcdbda8aa693ce2d08e12e4d23310bf2494d))
* **migrations:** add unique partial index for personal space workspace names ([a7d114e](https://github.com/sqlwarden/sqlwarden/commit/a7d114e34d67ecfd7f1e767b32b7430552cac362))
* **org:** add organization settings page ([646ebb3](https://github.com/sqlwarden/sqlwarden/commit/646ebb32c34e78b1cb947d8fa32febfd97b294df))
* **org:** remove placeholder home page ([36afd8d](https://github.com/sqlwarden/sqlwarden/commit/36afd8d9783eee052d4ecf7dc25c8c831805e90e))
* **orgs:** add member candidate search ([315e9a3](https://github.com/sqlwarden/sqlwarden/commit/315e9a30d35394e62a99d8989c603ff64388b51c))
* **query:** return backend execution duration in ResultSet, use it in results pane ([7e6744b](https://github.com/sqlwarden/sqlwarden/commit/7e6744b5d77925c708214d328294ceb58cb09a87))
* **rbac:** add environment-scope to ListAccessibleConnections, add ListAccessibleEnvironments ([460b7de](https://github.com/sqlwarden/sqlwarden/commit/460b7deb8d8b7ae2f89b6d5e5c15c3fedbbeaa1e))
* **rbac:** add workspace membership principals ([2350de8](https://github.com/sqlwarden/sqlwarden/commit/2350de8ad19e2a8c9a26b6ded93768761bbba729))
* **rbac:** connection inherits environment-scope bindings via dual hierarchy rows ([95e85f8](https://github.com/sqlwarden/sqlwarden/commit/95e85f8abe92052ecaa825d8158bea89c2d3f5e8))
* **rbac:** introduce workspace level roles, expand enforcer and accessible resources test, implement list connections/workspace apis ([a8a0fef](https://github.com/sqlwarden/sqlwarden/commit/a8a0fefa3dfd2f5c1a4044b3b6041b5566c84883))
* **rbac:** propagate ancestor visibility for discovery ([48e4cdc](https://github.com/sqlwarden/sqlwarden/commit/48e4cdc4a643a0d79d4af387373c19d5544fbac2))
* **rbac:** source permission metadata from api ([8ac2df8](https://github.com/sqlwarden/sqlwarden/commit/8ac2df8c44b89b4301078167004054991db73d12))
* **rbac:** use friendly builtin role constants ([f3f29de](https://github.com/sqlwarden/sqlwarden/commit/f3f29de57911ae09215ee141effeb3254bcf1bc8))
* **result:** add normalized ResultSet type ([6d55bc7](https://github.com/sqlwarden/sqlwarden/commit/6d55bc730d3fd775521bb721756ead6aaca92ed1))
* **schema:** add database schema introspection abstraction, cache, and API ([4e4d432](https://github.com/sqlwarden/sqlwarden/commit/4e4d432d27612468fc4d51eecfb638a002304f8c))
* **server:** support config driven https ([09b5d5e](https://github.com/sqlwarden/sqlwarden/commit/09b5d5eff8f24b6e9ab23dc5b7c373f4f63fecee))
* **setup:** create first organization during bootstrap ([9a29e73](https://github.com/sqlwarden/sqlwarden/commit/9a29e73d86a668f2bf3ffbfa6c7480b2ded742a1))
* **token:** add JWT access token and refresh token helpers ([ab16ba4](https://github.com/sqlwarden/sqlwarden/commit/ab16ba4dabfc2e4f3b7022aacda7d83eeb6fd3b8))
* **ui:** abstract and reuse table headers, search icon, debounced query, and show prerender loading ([1328630](https://github.com/sqlwarden/sqlwarden/commit/1328630200bfdd8ebdc903d053f2e08f2053d281))
* **ui:** abstract icon system with runtime pack switching (HugeIcons, Lucide, Remix) ([d2d312d](https://github.com/sqlwarden/sqlwarden/commit/d2d312de30bae41ec93ee244c59292b5a811b866))
* **ui:** add administration and instance management ([244bd0a](https://github.com/sqlwarden/sqlwarden/commit/244bd0a33356794f49f656edfd1412fd8bc26604))
* **ui:** add auth shell foundation ([7cda6de](https://github.com/sqlwarden/sqlwarden/commit/7cda6dee65defa687dca015635cc3783b94cb838))
* **ui:** add react project ([#28](https://github.com/sqlwarden/sqlwarden/issues/28)) ([3aef5e8](https://github.com/sqlwarden/sqlwarden/commit/3aef5e8c28000d31402eb0c31f94658d0ed2d2e5))
* **ui:** add resizable app sidebar ([417ed95](https://github.com/sqlwarden/sqlwarden/commit/417ed952817fb351ea0cab823fc445467b46894d))
* **ui:** add support for changing editor font style and size ([3dd94d6](https://github.com/sqlwarden/sqlwarden/commit/3dd94d64610bd8474c597f14b98c897319f88043))
* **ui:** add workspace policies page ([e34b13a](https://github.com/sqlwarden/sqlwarden/commit/e34b13a9a02b16726320bdec912bb0861ad70abf))
* **ui:** change design layout in pages and remove unnecessary files ([fff8ca5](https://github.com/sqlwarden/sqlwarden/commit/fff8ca5af288b733f61d0c60e2018006c5610160))
* **ui:** change font ([880ffa9](https://github.com/sqlwarden/sqlwarden/commit/880ffa985a57f4fe42f961ac005f8f56d5e76142))
* **ui:** change shadcn button to show pointer on hover ([e7ae443](https://github.com/sqlwarden/sqlwarden/commit/e7ae443dddd7e2c1ee6c1304acdd0e33c0ffb290))
* **ui:** change ui design system ([a8e6839](https://github.com/sqlwarden/sqlwarden/commit/a8e6839d55e489416e1a2f90444a6cd12a449e26))
* **ui:** change ui design system ([f24bddd](https://github.com/sqlwarden/sqlwarden/commit/f24bddde91780578f4165f6dd18bc5248779cba1))
* **ui:** consolidate access control into tabbed Members/Policies sections with consistent entity design ([6e6c633](https://github.com/sqlwarden/sqlwarden/commit/6e6c633ce88c3b7fd1d406a17370f6a16b5d975b))
* **ui:** hide menu items based on permissions, show toast on top center ([d5f2b8b](https://github.com/sqlwarden/sqlwarden/commit/d5f2b8b95cebe5219ca452fe9bd062272bcbaf88))
* **ui:** implement policies page for org level policies and workspace level policies ([e9101f4](https://github.com/sqlwarden/sqlwarden/commit/e9101f478d4725d0d61fc5bf7daf3cd5fabb73b1))
* **ui:** new preset test from shadcn ([ac3f1c6](https://github.com/sqlwarden/sqlwarden/commit/ac3f1c64a7ab77b01ce6c0805323e6a3b6f87f78))
* **users:** add instance user management ([96f7166](https://github.com/sqlwarden/sqlwarden/commit/96f71660b999be72899d8ef0b1a093b7a33fb081))
* **web:** seed local org for single-user mode ([c4aa9aa](https://github.com/sqlwarden/sqlwarden/commit/c4aa9aad945b5784aa61701a46f37fb5585ec6f7))
* **workspaces:** add effective member listing ([036a871](https://github.com/sqlwarden/sqlwarden/commit/036a871d4e1c73c7fae6a0ea52054fd13896f4ec))


### Bug Fixes

* **api:** align single resource reads with access lists ([d8bf1bc](https://github.com/sqlwarden/sqlwarden/commit/d8bf1bc7f81eca76ee318211b79559a223d8f3c8))
* **api:** bind query sessions to connection ([3cdb376](https://github.com/sqlwarden/sqlwarden/commit/3cdb376306809dd4e6bf9229537c90b8a9156669))
* **api:** block account registration until instance setup is complete ([c9ce40e](https://github.com/sqlwarden/sqlwarden/commit/c9ce40e54632431739d5715c6931500117a1f860))
* **api:** classify domain errors in handlers ([7aa9d64](https://github.com/sqlwarden/sqlwarden/commit/7aa9d64a461a77f80429474cd1d8426323f39735))
* **api:** enforce resource mutability rules ([d0ab9db](https://github.com/sqlwarden/sqlwarden/commit/d0ab9db70f6c38d0a40222895f7463f79c5c2fec))
* **api:** harden connection runtime access ([d93dd76](https://github.com/sqlwarden/sqlwarden/commit/d93dd769cf443ce43aea42e58173c62cba29a152))
* **api:** make errors json consistent with lowercase typing ([f7da30c](https://github.com/sqlwarden/sqlwarden/commit/f7da30cc9c3f402c1962d2e3d67dff7e54925b99))
* **api:** normalize ui-facing error contracts ([0b0a963](https://github.com/sqlwarden/sqlwarden/commit/0b0a9633291ba5d36fa69a299c22513f46d70222))
* **api:** require org membership for team joins ([45b3f86](https://github.com/sqlwarden/sqlwarden/commit/45b3f867fbc5b6060cbbfbc3bfcc7101184979f9))
* **api:** require policy read for workspace policies ([07df3dd](https://github.com/sqlwarden/sqlwarden/commit/07df3dd1041d7cb8c8daeb36f3bcf0f9f294727b))
* **api:** return 422 on duplicate org name instead of 500 ([b60954d](https://github.com/sqlwarden/sqlwarden/commit/b60954dfbf3f80740c4bae36fb2cf41d3361e3cd))
* **api:** satisfy audit checks ([17bf186](https://github.com/sqlwarden/sqlwarden/commit/17bf186222e1cacdf5e7f704a425c6191097f22b))
* **api:** standardize validation error messages ([f10907d](https://github.com/sqlwarden/sqlwarden/commit/f10907ddb52a52304c6658ef562f098cabe07a49))
* **auth:** clear user scoped query cache ([b0cafbb](https://github.com/sqlwarden/sqlwarden/commit/b0cafbba716722a4e759ae3e639e5878fa27dba4))
* **authz:** block deleting roles with active bindings ([a704671](https://github.com/sqlwarden/sqlwarden/commit/a704671b465bca900a03baf8dba3134c3ff3e72d))
* **ci:** allow upto 250 sized commit headers ([9c9354c](https://github.com/sqlwarden/sqlwarden/commit/9c9354c7fa8ae385d62b7c1e09669068d6acd3a5))
* **ci:** use correct output variable and add tag check ([#5](https://github.com/sqlwarden/sqlwarden/issues/5)) ([63acff9](https://github.com/sqlwarden/sqlwarden/commit/63acff9a2f92333cdb7b25373c1c292e2ee8d47b))
* **connection:** remove unused newMockOpen helper ([a75ba3d](https://github.com/sqlwarden/sqlwarden/commit/a75ba3d2bab0f7d28e06eb05133f472e0e284c21))
* **db:** fix hardcoded owner_type in InsertEnvironment and InsertConnection hierarchy rows ([c23f56d](https://github.com/sqlwarden/sqlwarden/commit/c23f56dc07a69d4555ffc55d5392b9d5cc7bd569))
* **deps:** pin tanstack router family to compatible latest versions ([c108867](https://github.com/sqlwarden/sqlwarden/commit/c1088670affd00f132a9187bf5b4a9aa73f20a0c))
* **driver:** fix go.sum for pgx/puddle, use strings.SplitSeq in mysql DSN parser ([c0a7aa0](https://github.com/sqlwarden/sqlwarden/commit/c0a7aa0ef644c4165d32aa50a3479ca8ebec8fdb))
* **frontend:** mark organization visit link as non-native button ([1c5f473](https://github.com/sqlwarden/sqlwarden/commit/1c5f473a5379071926b3781f13fd2e955336ac13))
* **frontend:** resolve tsc --noEmit type errors ([8bdadec](https://github.com/sqlwarden/sqlwarden/commit/8bdadec68e5e4bcdd4a3a5c155b6dbbfbff332e4))
* **hooks:** prevent panic in debugQueryLoggerHook when query result is nil ([d1daa6f](https://github.com/sqlwarden/sqlwarden/commit/d1daa6fd0043fbc56b0b4c975e038750d285f192))
* **ide:** compute console number from existing tabs instead of persistent counter ([1080b59](https://github.com/sqlwarden/sqlwarden/commit/1080b59cb406abb5aa16c961d5fac039cb17dd85))
* **ide:** cursor after semicolon or in blank line now runs preceding statement ([c720f21](https://github.com/sqlwarden/sqlwarden/commit/c720f21aa6c5c8de987f854b8e833a9b84f0cbd5))
* **ide:** handle null rows/columns in ResultSet for DML queries ([823c983](https://github.com/sqlwarden/sqlwarden/commit/823c983ee9966ce45918f1d67a7a9cff2ef00e14))
* **ide:** nativeButton=false on span triggers, replace onCollapse/onExpand with onResize ([9718199](https://github.com/sqlwarden/sqlwarden/commit/9718199d3581c26c41e0875ed53ab661ccb5c21a))
* **ide:** per-workspace tab isolation, Y.Doc lifecycle, and reload persistence ([bd5531a](https://github.com/sqlwarden/sqlwarden/commit/bd5531ad10ab278f0027490a2df07ae77b9d3076))
* **ide:** populate Y.Doc before SqlEditor mounts, always load file content on refresh ([44d15a9](https://github.com/sqlwarden/sqlwarden/commit/44d15a97dced750a49d08674fedd1260a201fe24))
* **ide:** preserve active tab on cross-window open/reload, fix spurious dirty flag on peer sync ([e49719e](https://github.com/sqlwarden/sqlwarden/commit/e49719e80ec203f118000d9889cf0859dcef1cce))
* **ide:** preserve unsaved indicator on tab switch, fix scroll reset on cell click ([8a48346](https://github.com/sqlwarden/sqlwarden/commit/8a483460b4ea1ae8b3eed2bb9bea15c803538e2f))
* **ide:** prevent newline on Mod-Enter by consuming key in CodeMirror keymap ([324ed70](https://github.com/sqlwarden/sqlwarden/commit/324ed7034b79ba5ab56ea0ec10bcea14cc529722))
* **ide:** prevent redundant file content fetches on tab switch ([aa98549](https://github.com/sqlwarden/sqlwarden/commit/aa9854988536f7ece0e5b8062961a7a5b431d325))
* **ide:** remove mismatched @codemirror/view keymap import, use window capture instead ([84125db](https://github.com/sqlwarden/sqlwarden/commit/84125dba5a83c01cfbd517b798598b7d6b1d838e))
* **ide:** remove monospace from connection placeholder, add collapsible sidebar toggle ([bebf5a8](https://github.com/sqlwarden/sqlwarden/commit/bebf5a82d59f8fcbd6312ac26a757962a8be2d40))
* **ide:** scope IndexedDB state per account to prevent cross-user data leakage ([669431e](https://github.com/sqlwarden/sqlwarden/commit/669431e1a63c6e8d348a10ea323067026d982835))
* **ide:** scope Y.js and store BroadcastChannels per account to prevent cross-user content sync ([88cad5a](https://github.com/sqlwarden/sqlwarden/commit/88cad5a16e14cc26502d8ecddd7b33d6462582d8))
* **ide:** sidebar toggle in toolbar, plain resize handle, workspace name in db pane ([1fd5359](https://github.com/sqlwarden/sqlwarden/commit/1fd53595aa22b39d84365342ffafa2a81d832a14))
* **ide:** swap run/connection layout, connection empty states, handle-click sidebar toggle ([f19515d](https://github.com/sqlwarden/sqlwarden/commit/f19515d7b134203a76e6d8b3779478b9c5e2b1e4))
* **ide:** use percentage strings for panel sizes (numbers were treated as px) ([406b344](https://github.com/sqlwarden/sqlwarden/commit/406b3447093a4aee6c235f445c7504c9785f4e57))
* **ide:** write empty content on file creation so first open returns 200 ([9c5a595](https://github.com/sqlwarden/sqlwarden/commit/9c5a5955a1a9960f44c383189ec914018bb5cdb9))
* **lint:** replace HasPrefix+TrimPrefix with CutPrefix in handlers_workspaces ([0014761](https://github.com/sqlwarden/sqlwarden/commit/0014761b8c5cf1bdeba3428da9af38266cacc03e))
* **org:** display organization name in sidebar header ([5c9c26c](https://github.com/sqlwarden/sqlwarden/commit/5c9c26c4390ca9209d61cc72054365c677041a74))
* **rbac:** prevent deleting last org owner policy ([6bf6762](https://github.com/sqlwarden/sqlwarden/commit/6bf67622bc3b36fd4ece598c6be9f98f340453f8))
* **rbac:** require org read for team directory ([26af453](https://github.com/sqlwarden/sqlwarden/commit/26af45336b2c5a84d586252c826bc31677b480e6))
* **rbac:** scope discovery by binding level ([33fa855](https://github.com/sqlwarden/sqlwarden/commit/33fa8557a4df41ce1ffc20fbbfe63f7febb42f34))
* **rbac:** separate effective resource permissions ([54f1493](https://github.com/sqlwarden/sqlwarden/commit/54f149320657a7b735aa4a970d8003fdef01b0bd))
* **result:** remove stale loop var copy, use any alias ([78e987a](https://github.com/sqlwarden/sqlwarden/commit/78e987a05ac73407c307b954c52c362702a98cf1))
* **roles:** show binding count on delete conflict, fix confirmation dialog copy ([79f3043](https://github.com/sqlwarden/sqlwarden/commit/79f304397de11dd625732243ac3a12abeb578307))
* **security:** gate sqlite target file connections ([4677ba6](https://github.com/sqlwarden/sqlwarden/commit/4677ba6a748ff6f7ba9a77191884b26b5cfb5581))
* **security:** protect owner-level org policy grants ([1710a5d](https://github.com/sqlwarden/sqlwarden/commit/1710a5d2263241ef8b8416df4e037d3216dfb04e))
* **style:** reformat with gofmt ([c546457](https://github.com/sqlwarden/sqlwarden/commit/c54645735c036e7a6ceb78a2630a0765031199a6))
* **tests:** use testcontainers singleton and bounded pools to fix connection exhaustion ([faaa1f9](https://github.com/sqlwarden/sqlwarden/commit/faaa1f92bddcad11b589a5950d855bcf65f7e553))
* **ui:** add overflow scrolling to dialogs, alert dialogs, popovers, and sheets ([d1c3883](https://github.com/sqlwarden/sqlwarden/commit/d1c3883878df1a1026f3e4834d78659cccd56876))
* **ui:** remove unnecessary pages ([59959f8](https://github.com/sqlwarden/sqlwarden/commit/59959f84e946d35327b5433060ee85f7a2f7d5ec))
* **ui:** show mobile sidebar trigger ([e53bc8d](https://github.com/sqlwarden/sqlwarden/commit/e53bc8dae103ba4b0d6a7b6515d0865be4c953d7))


### Performance Improvements

* **api:** use targeted access checks for single reads ([ba6d324](https://github.com/sqlwarden/sqlwarden/commit/ba6d324a06d4ba56f3927a280c291da032de7195))
* **frontend:** shrink bundle via icon subsetting, dev-only devtools, and font pruning ([f9370fc](https://github.com/sqlwarden/sqlwarden/commit/f9370fcf9aa998471d3a58541938fbe1e67c1571))


### Documentation

* add CLAUDE.md file ([bff2652](https://github.com/sqlwarden/sqlwarden/commit/bff2652cb246d4df4a9359fc38337f53550996a4))
* add rbac plans ([f5c8c14](https://github.com/sqlwarden/sqlwarden/commit/f5c8c14d0cac763bf89928ce970061a72e80e4d9))
* **api:** align contracts with strict hierarchy ([332df1f](https://github.com/sqlwarden/sqlwarden/commit/332df1fc2fc4b5d4ec82ecae42c196e0e27e7b84))
* **api:** finalize ui readiness contract ([740ea9d](https://github.com/sqlwarden/sqlwarden/commit/740ea9dd967ab3bdabceda6cd942d6cca67d0d8c))
* **architecture:** consolidate repository context ([900dcbc](https://github.com/sqlwarden/sqlwarden/commit/900dcbc8940ee2e90dff6982f6e6d1a5d454e6f7))
* **architecture:** refresh current implementation context ([913661d](https://github.com/sqlwarden/sqlwarden/commit/913661de6f38abea6f90f907507c54cad7ce61b2))
* archive stale implementation notes ([24dca20](https://github.com/sqlwarden/sqlwarden/commit/24dca203d47635961f32ed1ab8c4da3d1b538bd9))
* clarify org member and desktop access model ([cf7987b](https://github.com/sqlwarden/sqlwarden/commit/cf7987baf7cc087a3946eaa4384780650c0549b0))
* note pagination layering guidance ([c915f81](https://github.com/sqlwarden/sqlwarden/commit/c915f8101e75ce71a9fbaa8f347f636adfc8e5ed))
* remove reference rbac plan spec ([483ee22](https://github.com/sqlwarden/sqlwarden/commit/483ee227a92a512da18e1013f4bc37cf01c3e67d))
* update CLAUDE.md ([d22a6d0](https://github.com/sqlwarden/sqlwarden/commit/d22a6d0f1e28be37eb8a428d71e9eda28cc8f9d1))


### Code Refactoring

* **api:** drop legacy policy permission fields ([7e13879](https://github.com/sqlwarden/sqlwarden/commit/7e13879da13b9bcf987e01c92239d40b0d90047d))
* **api:** enforce strict environment connection hierarchy ([b1d864d](https://github.com/sqlwarden/sqlwarden/commit/b1d864d562cc2a3635494939a1f06ddcfee454a4))
* **api:** migrate config loading to viper ([a4bdb81](https://github.com/sqlwarden/sqlwarden/commit/a4bdb81f6fd64ac333823d948d30b09b9956d4b1))
* **api:** move paginated envelope to response ([421b021](https://github.com/sqlwarden/sqlwarden/commit/421b02116c2c5057805f1a49836ee3fb6a659484))
* **api:** move web app into internal package ([13eff89](https://github.com/sqlwarden/sqlwarden/commit/13eff8993177e47c464b514e83281f621b4c807f))
* **api:** normalize child resource ownership ([3750313](https://github.com/sqlwarden/sqlwarden/commit/3750313ddab82aaf232d09a149b82788b18c713f))
* **authz:** use explicit resource permission middleware ([77a749c](https://github.com/sqlwarden/sqlwarden/commit/77a749cafa331d33e4cb9b041968891c9ba19fae))
* **ci:** auto trigger goreleaser on release-please tag generation ([#19](https://github.com/sqlwarden/sqlwarden/issues/19)) ([1a76d4b](https://github.com/sqlwarden/sqlwarden/commit/1a76d4bf73ef9cb97e500b8a12cc4c823c097997))
* **ci:** consolidate goreleaser into release-please workflow ([#3](https://github.com/sqlwarden/sqlwarden/issues/3)) ([9016601](https://github.com/sqlwarden/sqlwarden/commit/9016601966a3f6cc2df6597a3b5c7258c2cb9b17))
* **ci:** standalone goreleaser workflow and change artifact name ([#7](https://github.com/sqlwarden/sqlwarden/issues/7)) ([ed9f314](https://github.com/sqlwarden/sqlwarden/commit/ed9f314c399fc71dc0655b7eb98f4942a78be845))
* **ci:** update workflow names for clarity and consistency ([#10](https://github.com/sqlwarden/sqlwarden/issues/10)) ([6db5f60](https://github.com/sqlwarden/sqlwarden/commit/6db5f60acd5c04577f573e6b62375a8ddb4aaaa1))
* **ci:** use github PAT token so goreleaser workflow is auto triggered ([1a76d4b](https://github.com/sqlwarden/sqlwarden/commit/1a76d4bf73ef9cb97e500b8a12cc4c823c097997))
* **db:** database methods to accept context.Context ([d395cc3](https://github.com/sqlwarden/sqlwarden/commit/d395cc3c1d044dc37965be46992703b5bafc4b10))
* **db:** drop unused binding list methods ([8290ac1](https://github.com/sqlwarden/sqlwarden/commit/8290ac15b327c7391e9d024193322ceba06a143b))
* **db:** inline simple paged list queries ([edfda97](https://github.com/sqlwarden/sqlwarden/commit/edfda979f922611727230b7aac4bb1c416eaac8f))
* **db:** remove redundant list methods ([a674cbd](https://github.com/sqlwarden/sqlwarden/commit/a674cbd240c1950eb3322dba8d9c7d408b0b6b17))
* **db:** unify workspace owner list path ([ffd9c80](https://github.com/sqlwarden/sqlwarden/commit/ffd9c803d7e779f602a9640701e99dda4d521a0d))
* **frontend:** adopt shadcn settings sidebar ([c27fcc9](https://github.com/sqlwarden/sqlwarden/commit/c27fcc937cc26e6b46f3a528202f46326ff0fef9))
* **frontend:** consolidate account and admin settings ([269c3d3](https://github.com/sqlwarden/sqlwarden/commit/269c3d326182b36493f146f283de6cca70cb9680))
* **frontend:** update settings app shell ([4582e26](https://github.com/sqlwarden/sqlwarden/commit/4582e26b213df57b4be12e8a4efb92da6114d335))
* **rbac:** make discovery permission-aware ([8e3ad19](https://github.com/sqlwarden/sqlwarden/commit/8e3ad19d13a18e471ac5f61eaa8ee4f266b72174))
* **rbac:** remove direct permission bindings ([b5d7737](https://github.com/sqlwarden/sqlwarden/commit/b5d773795a4441a8d9bba852c49a5aa5d6f2347b))
* **rbac:** simplify connection permissions ([f61baba](https://github.com/sqlwarden/sqlwarden/commit/f61baba13e6ee9bb2ea18337604743c08f0486f5))
* **rbac:** simplify organization builtin role names ([1187b96](https://github.com/sqlwarden/sqlwarden/commit/1187b96812cbcece2bf6002ba25e12b991f63b07))

## [0.5.0](https://github.com/sqlwarden/sqlwarden/compare/v0.4.0...v0.5.0) (2026-03-29)


### Features

* **db:** add support for both sqlite and postgres ([#27](https://github.com/sqlwarden/sqlwarden/issues/27)) ([62c127a](https://github.com/sqlwarden/sqlwarden/commit/62c127ab8482aa90f4c9e8e0a2e096636f85b209))
* **docker:** add docker image support and container registry publish ([#25](https://github.com/sqlwarden/sqlwarden/issues/25)) ([db477d0](https://github.com/sqlwarden/sqlwarden/commit/db477d04a20c3e25a39432b6f35766cd6ae388ee))
* **ui:** add react project ([#28](https://github.com/sqlwarden/sqlwarden/issues/28)) ([ed86d65](https://github.com/sqlwarden/sqlwarden/commit/ed86d6580f35c7ee3a2c0191dcd01b6ce5964801))

## [0.4.0](https://github.com/sqlwarden/sqlwarden/compare/v0.3.0...v0.4.0) (2026-02-22)


### Features

* **db:** change database layer to use pgx and sqlc for type-safe queries ([#23](https://github.com/sqlwarden/sqlwarden/issues/23)) ([1548947](https://github.com/sqlwarden/sqlwarden/commit/1548947d6cb09bcaabc9d2c40bb3d961d33bdfaf))
* **db:** migrate to pgx and sqlc for type-safe queries ([1548947](https://github.com/sqlwarden/sqlwarden/commit/1548947d6cb09bcaabc9d2c40bb3d961d33bdfaf))

## [0.3.0](https://github.com/sqlwarden/sqlwarden/compare/v0.2.5...v0.3.0) (2026-02-22)


### Features

* initial commit - project bootstrapped from autostrada.dev ([7d3ec4e](https://github.com/sqlwarden/sqlwarden/commit/7d3ec4e05361c693a1c7cfd0a99fc6d0895375a8))


### Bug Fixes

* **ci:** update name_template to not use capitalized OS names ([0133919](https://github.com/sqlwarden/sqlwarden/commit/0133919315ad288cf15de3d31a4485fcb1db1e1a))
* **ci:** use correct output variable and add tag check ([8c9dc74](https://github.com/sqlwarden/sqlwarden/commit/8c9dc749a42db420f6d1ea52fa0d698fea4b9293))
* **ci:** use correct output variable and add tag check ([#5](https://github.com/sqlwarden/sqlwarden/issues/5)) ([dec890f](https://github.com/sqlwarden/sqlwarden/commit/dec890f0e25c6e120489658aaa39695f47e745aa))


### Code Refactoring

* **ci:** auto trigger goreleaser on release-please tag generation ([#19](https://github.com/sqlwarden/sqlwarden/issues/19)) ([341ba62](https://github.com/sqlwarden/sqlwarden/commit/341ba6270699eec470cbb7011be17e0f2995753f))
* **ci:** consolidate goreleaser into release-please workflow ([4123c0a](https://github.com/sqlwarden/sqlwarden/commit/4123c0a5b9d7c3037f0396e844647ec573b1430b))
* **ci:** consolidate goreleaser into release-please workflow ([#3](https://github.com/sqlwarden/sqlwarden/issues/3)) ([7ee8ad2](https://github.com/sqlwarden/sqlwarden/commit/7ee8ad26acc1e84400cd72523e3373641722955f))
* **ci:** standalone goreleaser workflow and change artifact name ([#7](https://github.com/sqlwarden/sqlwarden/issues/7)) ([21e605b](https://github.com/sqlwarden/sqlwarden/commit/21e605b9195a7cfd9ff17812500f07e2da8a261e))
* **ci:** update workflow names for clarity and consistency ([d2075b0](https://github.com/sqlwarden/sqlwarden/commit/d2075b0d93eccd7b0793da9e0fe332078c77a161))
* **ci:** update workflow names for clarity and consistency ([#10](https://github.com/sqlwarden/sqlwarden/issues/10)) ([b5a250b](https://github.com/sqlwarden/sqlwarden/commit/b5a250b2aa702dcd8ee31f66b9279b19a6f5e0eb))
* **ci:** use github PAT token so goreleaser workflow is auto triggered ([341ba62](https://github.com/sqlwarden/sqlwarden/commit/341ba6270699eec470cbb7011be17e0f2995753f))

## [0.2.5](https://github.com/sqlwarden/sqlwarden/compare/v0.2.4...v0.2.5) (2026-02-22)


### Code Refactoring

* **ci:** auto trigger goreleaser on release-please tag generation ([#19](https://github.com/sqlwarden/sqlwarden/issues/19)) ([341ba62](https://github.com/sqlwarden/sqlwarden/commit/341ba6270699eec470cbb7011be17e0f2995753f))
* **ci:** use github PAT token so goreleaser workflow is auto triggered ([341ba62](https://github.com/sqlwarden/sqlwarden/commit/341ba6270699eec470cbb7011be17e0f2995753f))

## [0.2.4](https://github.com/sqlwarden/sqlwarden/compare/v0.2.3...v0.2.4) (2026-02-04)


### Code Refactoring

* **ci:** update workflow names for clarity and consistency ([d2075b0](https://github.com/sqlwarden/sqlwarden/commit/d2075b0d93eccd7b0793da9e0fe332078c77a161))
* **ci:** update workflow names for clarity and consistency ([#10](https://github.com/sqlwarden/sqlwarden/issues/10)) ([b5a250b](https://github.com/sqlwarden/sqlwarden/commit/b5a250b2aa702dcd8ee31f66b9279b19a6f5e0eb))

## [0.2.3](https://github.com/sqlwarden/sqlwarden/compare/v0.2.2...v0.2.3) (2026-02-04)


### Bug Fixes

* **ci:** update name_template to not use capitalized OS names ([0133919](https://github.com/sqlwarden/sqlwarden/commit/0133919315ad288cf15de3d31a4485fcb1db1e1a))


### Code Refactoring

* **ci:** standalone goreleaser workflow and change artifact name ([#7](https://github.com/sqlwarden/sqlwarden/issues/7)) ([21e605b](https://github.com/sqlwarden/sqlwarden/commit/21e605b9195a7cfd9ff17812500f07e2da8a261e))

## [0.2.2](https://github.com/sqlwarden/sqlwarden/compare/v0.2.1...v0.2.2) (2026-02-04)


### Bug Fixes

* **ci:** use correct output variable and add tag check ([8c9dc74](https://github.com/sqlwarden/sqlwarden/commit/8c9dc749a42db420f6d1ea52fa0d698fea4b9293))
* **ci:** use correct output variable and add tag check ([#5](https://github.com/sqlwarden/sqlwarden/issues/5)) ([dec890f](https://github.com/sqlwarden/sqlwarden/commit/dec890f0e25c6e120489658aaa39695f47e745aa))

## [0.2.1](https://github.com/sqlwarden/sqlwarden/compare/v0.2.0...v0.2.1) (2026-02-04)


### Code Refactoring

* **ci:** consolidate goreleaser into release-please workflow ([4123c0a](https://github.com/sqlwarden/sqlwarden/commit/4123c0a5b9d7c3037f0396e844647ec573b1430b))
* **ci:** consolidate goreleaser into release-please workflow ([#3](https://github.com/sqlwarden/sqlwarden/issues/3)) ([7ee8ad2](https://github.com/sqlwarden/sqlwarden/commit/7ee8ad26acc1e84400cd72523e3373641722955f))

## [0.2.0](https://github.com/sqlwarden/sqlwarden/compare/v0.1.0...v0.2.0) (2026-02-04)


### Features

* initial commit - project bootstrapped from autostrada.dev ([7d3ec4e](https://github.com/sqlwarden/sqlwarden/commit/7d3ec4e05361c693a1c7cfd0a99fc6d0895375a8))
