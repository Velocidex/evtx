package evtx

type MessageResolverOpts struct {
	// A regular expression that if matched, will place the language
	// first in the list of languages.
	// This defaults to "en-(US|AU|GB)"

	// https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/available-language-packs-for-windows?view=windows-11
	LangPreferenceRegeExp string

	// Size of Message LRU - defaults to 100
	LRUSize int
}
