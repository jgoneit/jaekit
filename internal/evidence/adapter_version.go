package evidence

// adapterSemantics is each adapter's classification meaning version. It is
// part of an adapter declaration's binding digest: raising a version makes
// every earlier run of that adapter stale instead of reinterpreting it.
var adapterSemantics = map[string]string{"go-test-json": "1", "junit-xml": "1"}
