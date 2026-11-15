//go:build goalcheckcleanup

package cleanupchecks

// Expected values written from the tree at the goal's base. A maintain
// condition compares the checked tree with these values; it never reads the
// base commit.

type expectedFile struct {
	sha256     string
	executable bool
}

// Every tracked file of the five kept folders (AC-1).
var keptFolderFiles = map[string]expectedFile{
	"tools/goal-checks/pages-site/AC-1.json":                                    {"54f6d68716ff4c31e06863cf90e51ffafff1c83627fddc9e44f43e8fa3b6af5c", false},
	"tools/goal-checks/pages-site/AC-10.json":                                   {"6dead18a1c4eccc773c58aa835d3f3f0da79f5de5610df8c0e4f09f7b958873b", false},
	"tools/goal-checks/pages-site/AC-11.json":                                   {"5b396a8f14ccbaa201bc89e684c43d523504e3eb48376e7f32e2052529394449", false},
	"tools/goal-checks/pages-site/AC-12.json":                                   {"3a810c129ba5b358d12b2bcb786883fa93059fce673420402532e05a0284b04d", false},
	"tools/goal-checks/pages-site/AC-14.json":                                   {"a696e815fd5e3e92b3b575e48ca6d694143b733f42b69efe1c6693966cf85864", false},
	"tools/goal-checks/pages-site/AC-2.json":                                    {"028e437ba908ebc6825f955e1a6b6156a672f324cfc2312927b9c1a58bb576c0", false},
	"tools/goal-checks/pages-site/AC-3.json":                                    {"4c28cfe62269268746fb6f47313776cb9ef770fd27ed3b9da78465ecf50727f3", false},
	"tools/goal-checks/pages-site/AC-4.json":                                    {"c82268e565e2dc10bd01d2909d06595e6e6678a86d74d033682446b6038de058", false},
	"tools/goal-checks/pages-site/AC-5.json":                                    {"97339be92abf155a9f858777ce68eda88dd4549899dad2aa56a0ac4580951f34", false},
	"tools/goal-checks/pages-site/AC-6.json":                                    {"f49b3561ece3fa8d014c970be59e825ff62e24dfcbaae63c5102e59443d7434f", false},
	"tools/goal-checks/pages-site/AC-7.json":                                    {"305e1267c6b6ba912bc350523f567448c8cfb6d38dea1fc3f7a5a9304ec4add2", false},
	"tools/goal-checks/pages-site/AC-8.json":                                    {"f63b07ca7da9bb25e9a01076c832915a0c79b52eb0d00b0298caf486ff3270d3", false},
	"tools/goal-checks/pages-site/AC-9.json":                                    {"9a04b7b8b20de3b9cb4a322eaa00db8c6fd274745a364db469072a794dc4f83a", false},
	"tools/goal-checks/pages-site/helpers_test.go":                              {"fdbc5ea566af2f6e869985fa65a31ca7e2dd3b5f00b46c2faa3da2d9e4141bb4", false},
	"tools/goal-checks/pages-site/site_test.go":                                 {"c46d3e6121f73f799c8f63837d175bb6602584190a6b360a7dc1416a09a57ff2", false},
	"tools/goal-checks/release-0-1-3-followups/AC-1.declaration.json":           {"189d2f5dc4d761bb4db31fee0972be14b69f9d66edb00574ff82fc13a65c21cf", false},
	"tools/goal-checks/release-0-1-3-followups/AC-1.targets.json":               {"e1c0ef9468e70600b505aebfc396156a3f59ed8dc386787c6c9d9bf31d6a3246", false},
	"tools/goal-checks/release-0-1-3-followups/AC-10.declaration.json":          {"db751ef5abd6c153cacbbb15581a622a1a58760079e29913c94808f97166789a", false},
	"tools/goal-checks/release-0-1-3-followups/AC-10.targets.json":              {"39c41b40d1bc045d67770cb9b8cfe495d49a1cc2fb15e261e58ec6b81a35a9a7", false},
	"tools/goal-checks/release-0-1-3-followups/AC-11.declaration.json":          {"f720b3d3c39ab1445d61330ec3d9742ee745931dea42a4d521d70d3c302655f1", false},
	"tools/goal-checks/release-0-1-3-followups/AC-11.targets.json":              {"36c31a1e20d8f2b7f91f6856b509f30afd970d4f7f6258d6450d605e4f03a27a", false},
	"tools/goal-checks/release-0-1-3-followups/AC-2.declaration.json":           {"fec30f0057f1b0f75b643421944a18efecdc4f792917a5c25d892e7c7a293bb8", false},
	"tools/goal-checks/release-0-1-3-followups/AC-2.targets.json":               {"80f9708e3716c159639de6b7935be3dd427b0543b0356ca1f770be7490b01094", false},
	"tools/goal-checks/release-0-1-3-followups/AC-3.declaration.json":           {"182d507e4dbff74f6d0b2bb39b27d27e1e048ef21a4f81b02a23d518a5be8baa", false},
	"tools/goal-checks/release-0-1-3-followups/AC-3.targets.json":               {"9f8d2cd35e1c4ecb2bcc8aed8b26db1bf5a8f42331a8398fba5eb2215203c447", false},
	"tools/goal-checks/release-0-1-3-followups/AC-4.declaration.json":           {"a6190c814418aae67db09868675e6fae8a228ecb69a49e7b57e8152a1e82f36a", false},
	"tools/goal-checks/release-0-1-3-followups/AC-4.targets.json":               {"6e9e595d17068b2e2f2c3506613049c3e26acfab1949c8589fc4937f3b41fce7", false},
	"tools/goal-checks/release-0-1-3-followups/AC-5.declaration.json":           {"334ac0f505887fb10422ba2a90fd164b5433d0aa84727a7adec1b389899f0da5", false},
	"tools/goal-checks/release-0-1-3-followups/AC-5.targets.json":               {"357140fb4826cfa849ee392d3b72cb89645d19e5e429dbfafba49658e9933026", false},
	"tools/goal-checks/release-0-1-3-followups/AC-6.declaration.json":           {"6a767105af106400ad9aceb58adaae0623121101dd775fce528df21d65fccdca", false},
	"tools/goal-checks/release-0-1-3-followups/AC-6.targets.json":               {"06a056b6454c1ead1689cbfe05004b6ed810c57656852c3d3a11f27c97084f29", false},
	"tools/goal-checks/release-0-1-3-followups/AC-7.declaration.json":           {"589b3808386f57585f7b4ab6a1ce8c1713ed8dd940727171242d9db9caeef6de", false},
	"tools/goal-checks/release-0-1-3-followups/AC-7.targets.json":               {"11209429c6c96bfb58fa0e9b37d94c368bf1a6080d88c4485d8538a299012cce", false},
	"tools/goal-checks/release-0-1-3-followups/AC-8.declaration.json":           {"a1622984d573f63326ad6ff7b11d9acf76eb32a9d4a50dc3a9dede6d4954d81b", false},
	"tools/goal-checks/release-0-1-3-followups/AC-8.targets.json":               {"477758b3e1cc82be4a1ac98c54a79e11725e701a955abaeeb50057bdb3dedabb", false},
	"tools/goal-checks/release-0-1-3-followups/AC-9.declaration.json":           {"153ff5be479dae1ac139b1791089784d64a8b43a49614cfcec05487286981051", false},
	"tools/goal-checks/release-0-1-3-followups/AC-9.targets.json":               {"d035548b9e3894715c5e8c7d711dc18df79167113c2c1d3096ffadc2dd6f27c6", false},
	"tools/goal-checks/release-0-1-3-followups/preserved.json":                  {"1094dc6bf295dc2e65a34662d0c9dc0606089bc679e072d9d6a8bd121d5679a5", false},
	"tools/goal-checks/release-0-1-3-followups/preserved.py":                    {"da93ee5449f5cf03b87574faf2a3891abc30facce82bdeb289443ebd5b038b36", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-1.declaration.json":        {"54903d792126852a39025d2eeabd96b6d1ab07705f4ce81c788e3359e53d13b1", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-1.targets.json":            {"7d0ac371ad55cedb5eab0b55824e7acd5ef2868a802ff07bd592c74ef5cf125c", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-10.declaration.json":       {"ca55dbd94d48abf0576921bbf2215a64ffb98c418ca1a46216060ee92d169b2a", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-10.targets.json":           {"2e177b4b36e430a78fa2f1a3fc0aca9fc3c542941f696b4f76de39f32282ae76", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-11.declaration.json":       {"8f63bc6af6633b1e388259042901ac3e4a6870c096eb23ed8fe38c417969a1ca", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-11.targets.json":           {"8c536a0e545866065b92ae3c4b0ab87b553163a992b64623296089801f0ffd81", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-2.declaration.json":        {"616333f81d993d82c896e0cc42422612d0d76952f197041df1488681e9c95e59", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-2.targets.json":            {"8fa7d5ea5e89441e62786dc33143a6060797295e2152fdbd15fe52a199b80f64", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-3.declaration.json":        {"80813dcababa1baf5aa4bee11e53c9087e8c98bde4afe0f3e783db623b5c4cf6", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-3.targets.json":            {"ec3c38e7ae169af02136b9d3bc24d597667bd97e667f54d5bbdb4fd2ca406e60", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-4.declaration.json":        {"c9485491f0f74cd8faa4867d126cbfe1e79bfba9bebf7455d988cac9812603e8", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-4.targets.json":            {"5581771dcc788c4773f7dd5cbb64ab7110767ef9ad591a533df81b1a1ac0c886", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-5.declaration.json":        {"73427fdba1f826ab5b59c1c66ca5e6ecdee18cc870414ecffd23231f294a36d6", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-5.targets.json":            {"3954881768b15f12b4885fb12c4face253c212c6a60e34b796db998e0b47da4e", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-6.declaration.json":        {"63aaae7fd54a262c64122f26e4685c8f02f5d2a8a9f1bfbebbca091eec4b8666", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-6.targets.json":            {"a1afcc92458eecea1af28969644dbae145e242eecafd54d8977c2d52b863c65d", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-7.declaration.json":        {"7e57e69bb0019265fdc75167df3034d454a1a3658c74c00c9e5f7a1319364bfd", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-7.targets.json":            {"a653053b9ddab8502df542b631d84278840b8a7163b2e6c4e44f241647b22908", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-8.declaration.json":        {"9c3a7feddebc23beb02bbb44f7ef179b1c4316679d85dcb3889372859d60e8e9", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-8.targets.json":            {"785603c77155c92797b7e89b50f282c4725e637d13a752ebbb4b5ccd950213b2", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-9.declaration.json":        {"ae46a499f5c864a355bd5a4fa15f7bc01c79db9c809ace44e8de263ffbc223d8", false},
	"tools/goal-checks/release-0-1-3-review-fixes/AC-9.targets.json":            {"89d96bde5bf70aa20b894afa6c62639c5a0a82024930828177663a3c46326363", false},
	"tools/goal-checks/release-0-1-3-review-fixes/test_private_independence.py": {"a354fcad1f07c682a08d7cbe0d7e9fe528467d3008a6de34854b0b583dfaecab", false},
	"tools/goal-checks/review-findings/AC-1.declaration.json":                   {"db441fb0e057ce168cd4717d2641f4b04c3c2f6131f84403b08a0159379fe270", false},
	"tools/goal-checks/review-findings/AC-1.targets.json":                       {"738b221de0154fe6c5a4ff90b95659cf00fcbf69a0b5804b2f86394d5b68e3a4", false},
	"tools/goal-checks/review-findings/AC-2.declaration.json":                   {"071edf02a7a7b28bbb01bb3571131a5cf80b0396bad267e14cf8bf6c9992cdb6", false},
	"tools/goal-checks/review-findings/AC-2.targets.json":                       {"71b9d194709beba6a4af995a464e6194837043bcfc32af933f17a2a845812bf3", false},
	"tools/goal-checks/review-findings/AC-3.declaration.json":                   {"6a1bb5f61dd2bf826428352c749283b1eacba3ff7292b73a74a38cd5444d7185", false},
	"tools/goal-checks/review-findings/AC-3.targets.json":                       {"79861dd8b2bc6d1a9fe1c3078525be2fe622d93f7db6584a308688e105e1214c", false},
	"tools/goal-checks/review-findings/AC-4.declaration.json":                   {"956e682fe4303e62714f33dae24aeef3cfa32547aed2dbd1d92a1f0e54cec851", false},
	"tools/goal-checks/review-findings/AC-4.targets.json":                       {"58bb5d6db0d5c51fde06c5e2acbe29872192dcfe03c96776231f65b2b6e95301", false},
	"tools/goal-checks/review-findings/AC-5.declaration.json":                   {"b2327ab3ef3c33211976b672a0db36c1fe4245f04b6fe041460c36c26ade2e2a", false},
	"tools/goal-checks/review-findings/AC-5.targets.json":                       {"f3a2201ed0d9fdd22eca2ec204bf1c224b3dacbe53cbbb470439c60dd99cae33", false},
	"tools/goal-checks/review-findings/AC-6.declaration.json":                   {"71edb9096e0198e719831160181720cbc56bd96e43db6c851fb0c05123a185e5", false},
	"tools/goal-checks/review-findings/AC-6.targets.json":                       {"e4bb7fd9f20d55a560dc18bf9f1bde85a88391baf1caf732bf940da4ade666b7", false},
	"tools/goal-checks/windows-entry-name-refusal/AC-2.declaration.json":        {"8fb8615ff2674e1ad23eabc5bcaf0986505b3ecb2c5dee0e3a88c79938a9b440", false},
	"tools/goal-checks/windows-entry-name-refusal/AC-2.targets.json":            {"b6a4b20e044601e0df5e4bb3f2a2d8da9075cd77be2fefa56c68321499421dce", false},
	"tools/goal-checks/windows-entry-name-refusal/ci_runs.py":                   {"b4ca50892c4a75489b40aa5e8c4aae0ea9b6890f30425c806bbdcee0cf88c1a1", false},
}

// Every tracked file directly under tools/goal-checks (AC-3).
var rootFiles = map[string]expectedFile{
	"tools/goal-checks/AC-1.json":                  {"ab4f9fae079dbce23617d886d2ade93063635ffb078db3f40f7f304db08b4bdb", false},
	"tools/goal-checks/AC-1.retention.json":        {"28eaee16bbe83235e7c63ccb85b5936924a981115b614e99bbb6a11d41bffe6b", false},
	"tools/goal-checks/AC-10.json":                 {"e13b41388d9168cdc35970f5315c0313fe1d520bbba78a45e52df831f44667b9", false},
	"tools/goal-checks/AC-11.json":                 {"2e0f1f924cf480adf4d346883a4e58e9bcc1b8744e5a6bee794adbc655476c63", false},
	"tools/goal-checks/AC-2.json":                  {"79dd62b314eb7b052ec2e5aa8e70759b175d96b53440e4baefb6b19d5437a4a9", false},
	"tools/goal-checks/AC-2.retention.json":        {"b083fc6065ee1377b48e519d23a7e2e6b5e6aeff685b5af74c27a3a19c8d0556", false},
	"tools/goal-checks/AC-3.json":                  {"8088f95ce9f5914197f2ef34f9f55920e38a094fd0a1046467a31f30fea11698", false},
	"tools/goal-checks/AC-3.retention.json":        {"ca67da8af0319bcf49e017110acfb8330d88c178875b79a3367609924d9b6ae6", false},
	"tools/goal-checks/AC-6.json":                  {"69e777c2e8f30f836ced37299bfcb0698a764a08c9a528e850e363b3bcef792a", false},
	"tools/goal-checks/AC-7.json":                  {"29ad05c2ef3e875201da16fd2884e1687589a793b16923a7a213ff5b4d963ba9", false},
	"tools/goal-checks/AC-8.json":                  {"c8c42529aeffe292a9052e18f7f480e1d50cf2dc3f501018065e5beb828a9412", false},
	"tools/goal-checks/README.md":                  {"d64a5eaa48fd91fd5b082293e9f67941e1e56b173ea44057747ce2bf2d2bb5b3", false},
	"tools/goal-checks/go-result.py":               {"b7297e771cba1d25de7de0450774cf71b591509dcc849b28d9956d4260d58ae8", false},
	"tools/goal-checks/produce.py":                 {"4e86de7d422c17c6249e7fd3e222a99f07b90deb5e47f501c1ddbe6bc3c2a7e3", false},
	"tools/goal-checks/retention_evidence.py":      {"31fd120a5e1cda6f9383be31a1e2324f29f68b6446ed3cd93ab0a65ccad55a08", false},
	"tools/goal-checks/test_authoring_check.py":    {"b713aa8db806b020416c3d5395ea5134b805c0f10b839d9d4540da66c30706fb", false},
	"tools/goal-checks/test_entry_name_ci_runs.py": {"cf6c310d773e503f61a6cbb9dd1df79ab33d3a71fdd10a5bf79589c37123b714", false},
	"tools/goal-checks/test_go_result.py":          {"eda2485c6f368ab5ee5d9af03284e2ee5726d218c8e5ab8a8a248b104d9afa33", false},
	"tools/goal-checks/test_history_audit.py":      {"767bc95e2b50aeccd11275c17ea1c92e15f7b82f45ebe1d18d45023f7c902326", false},
	"tools/goal-checks/test_retention.py":          {"c8b40535cd2b17381754b00478d5090dbc2543f01428e6991221c873374974ba", false},
}

// The protected plugins tests and the regression map (AC-5).
var protectedFiles = map[string]expectedFile{
	"contracts/regressions.json":                {"4c027b6909f43b4d1963b969f319b826f2b91412292dd4f7c7bc222eeec6fc4e", false},
	"plugins/spec_boundaries_contract_test.go":  {"627eeeebcf9bf99f93fb86f1b43454e4c94830fdc9c21efb7eff6d48e8b088fb", false},
	"plugins/verification_design_test.go":       {"665ee6913d594f5c94395df1e0cfe83e386f7d20f0d0e3f14082ae0762d984b8", false},
	"plugins/verification_finding_docs_test.go": {"856639dfcf0344472f8d17c0100bd0bd3f46ea1ce508af42ff5b50e6651ab59e", false},
	"plugins/verification_reporting_test.go":    {"091428c3b8887a8bb450618cdeb50dd1b07392561470f2f13a8e13381da64593", false},
}

// Development guides with the Seal word-cap sentence's test links masked (AC-9).
var developmentGuides = map[string]string{
	"guides/DEVELOPMENT.md":    "29565242794e7d1d2e86f7ae025315fc8c54c82e5de4f5955d42415200e7ea04",
	"guides/DEVELOPMENT.en.md": "cdcd6d4772b4b5e8c6240229c15d033aef3c625e42978e72ef9eb730829007c4",
}

// Bodies of the tests other tracked files run by name (AC-13).
var namedTestBodies = map[string]string{
	"TestRelease011UninstallGuide":    "5b45e8a0091141f1c7df671d9e3a2e4b078c37dc65be5a4c2c82ab07ee9f5cd9",
	"TestRelease011UpdateGuide":       "3e5130a839fdca654439bfc363ed1a59b59ea1e3b64bbda73071f8957f34d8c4",
	"TestRelease013ReferenceGuidance": "a8017c49441bbb7f696a0db1e130c4cf750d3d060f3e94f78e15e9c5afbd062c",
}
