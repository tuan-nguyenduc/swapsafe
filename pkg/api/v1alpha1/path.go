package v1alpha1

import "strings"

// regexMeta are the characters that make an Ingress path a pattern rather
// than a path. ingress-nginx reads a path as a regular expression when
// use-regex or rewrite-target is set, and such a path cannot be requested.
const regexMeta = `(.*+?[]{}^$|\`

// LiteralPathPrefix returns the part of an Ingress path that is a path: the
// text up to the first regular expression metacharacter.
//
//	/shop                 -> /shop
//	/podinfo(/|$)(.*)     -> /podinfo
//	/api/v([0-9]+)/users  -> /api/v
//	/(.*)                 -> /
//
// It is what makes the prefix-strip idiom pairable with the PathPrefix the
// translation produces, and what gives the probes a URL that can be sent.
func LiteralPathPrefix(path string) string {
	if cut := strings.IndexAny(path, regexMeta); cut >= 0 {
		path = path[:cut]
	}
	return NormalizePath(path)
}

// NormalizePath drops a trailing slash so that "/shop" and "/shop/" are one
// path. Whether the two endpoints agree about the slash is a behaviour
// question for the differ, not a reason to call two routes different.
func NormalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	if path == "" {
		return "/"
	}
	return path
}

// PathPrefixCovers reports whether a Gateway API PathPrefix value serves
// path. Gateway API matches a prefix on whole segments, so /api covers
// /api/v1 but not /apidocs.
func PathPrefixCovers(prefix, path string) bool {
	prefix, path = NormalizePath(prefix), NormalizePath(path)
	return prefix == "/" || prefix == path || strings.HasPrefix(path, prefix+"/")
}
