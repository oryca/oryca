package trie

import (
	"github.com/oryca/oryca/gateway/model"
	"regexp"
	"strings"
	"sync"
)

type TrieNode struct {
	children   map[string]*TrieNode
	paramChild *TrieNode
	paramName  string // name of paramChild's path param, "collectionId" for example
	isEnd      bool
	service    *model.GatewayService
	resource   *model.ResourcePath

	// A wildcard suffix (a path ending in /*) anchored at this node. Always kept apart from the
	// isEnd/service/resource fields above, because an exact match that ends here ("/items") and a
	// wildcard that starts here ("/items/*") are different resources. Sharing the fields would let
	// whichever was inserted second overwrite the other. This was a real bug, where one resource's
	// destination was overwritten by another whose path segments overlapped.
	hasWildcard      bool
	wildcardService  *model.GatewayService
	wildcardResource *model.ResourcePath
}

type PathTrie struct {
	root *TrieNode
	mu   sync.RWMutex
}

type PathMatch struct {
	Service    *model.GatewayService
	Resource   *model.ResourcePath
	PathParams map[string]string
	Remaining  string
}

func New() *PathTrie {
	return &PathTrie{
		root: &TrieNode{children: make(map[string]*TrieNode)},
	}
}

func isPathParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func paramName(segment string) string {
	return segment[1 : len(segment)-1]
}

func (pt *PathTrie) Insert(svc *model.GatewayService) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	for _, res := range svc.ResourcePaths {
		pt.insertResource(svc, res)
	}
}

func (pt *PathTrie) insertResource(svc *model.GatewayService, res *model.ResourcePath) {
	fullPath := strings.TrimRight(svc.BasePath, "/") + "/" + strings.TrimLeft(res.Path, "/")
	segments := strings.Split(strings.Trim(fullPath, "/"), "/")
	current := pt.root

	for _, seg := range segments {
		if seg == "" {
			continue
		}
		// wildcard suffix. Mark this node as a wildcard anchor, separately from any exact match on
		// the same node (another resource may end exactly here), then stop
		if seg == "*" {
			current.hasWildcard = true
			current.wildcardService = svc
			current.wildcardResource = res
			return
		}
		if isPathParam(seg) {
			if current.paramChild == nil {
				current.paramChild = &TrieNode{children: make(map[string]*TrieNode)}
				current.paramName = paramName(seg)
			}
			current = current.paramChild
		} else {
			if current.children[seg] == nil {
				current.children[seg] = &TrieNode{children: make(map[string]*TrieNode)}
			}
			current = current.children[seg]
		}
	}

	current.isEnd = true
	current.service = svc
	current.resource = res
}

func (pt *PathTrie) Delete(serviceID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.deleteFromNode(pt.root, serviceID)
}

func (pt *PathTrie) deleteFromNode(node *TrieNode, serviceID string) {
	if node == nil {
		return
	}
	if node.isEnd && node.service != nil && node.service.ID == serviceID {
		node.isEnd = false
		node.service = nil
		node.resource = nil
	}
	if node.hasWildcard && node.wildcardService != nil && node.wildcardService.ID == serviceID {
		node.hasWildcard = false
		node.wildcardService = nil
		node.wildcardResource = nil
	}
	for _, child := range node.children {
		pt.deleteFromNode(child, serviceID)
	}
	if node.paramChild != nil {
		pt.deleteFromNode(node.paramChild, serviceID)
	}
}

func (pt *PathTrie) LoadBulk(services []*model.GatewayService) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.root = &TrieNode{children: make(map[string]*TrieNode)}
	for _, svc := range services {
		for _, res := range svc.ResourcePaths {
			pt.insertResource(svc, res)
		}
	}
}

func (pt *PathTrie) FindBestMatch(requestPath string) *PathMatch {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	segments := strings.Split(strings.Trim(requestPath, "/"), "/")
	return pt.search(pt.root, segments, 0, map[string]string{}, nil)
}

func (pt *PathTrie) search(node *TrieNode, segments []string, idx int, params map[string]string, resourcePath *model.ResourcePath) *PathMatch {
	if idx >= len(segments) {
		if node.isEnd {
			paramsCopy := make(map[string]string, len(params))
			for k, v := range params {
				paramsCopy[k] = v
			}
			return &PathMatch{
				Service:    node.service,
				Resource:   node.resource,
				PathParams: paramsCopy,
			}
		}
		// No exact resource on this node, but if a wildcard is anchored here (a resource declared as
		// plain "/*", with no other path of its own) then a bare path matching the parent of "*"
		// should match too. Exact still wins. That was checked above. So this is only a fallback
		// for when no exact match competes on the same node.
		if node.hasWildcard {
			paramsCopy := copyParams(params)
			return &PathMatch{
				Service:    node.wildcardService,
				Resource:   node.wildcardResource,
				PathParams: paramsCopy,
			}
		}
		return nil
	}

	seg := segments[idx]

	// case 1: exact match, highest priority
	if child, ok := node.children[seg]; ok {
		if m := pt.search(child, segments, idx+1, params, resourcePath); m != nil {
			return m
		}
	}

	// case 2: pattern match, {y}.png for example
	for childSeg, child := range node.children {
		if strings.Contains(childSeg, "{") && strings.Contains(childSeg, ".") {
			if matchesPattern(seg, childSeg) {
				// only copy once the pattern has matched
				newParams := copyParams(params)
				newParams[extractPatternParamName(childSeg)] = extractParamFromPattern(seg)
				if m := pt.search(child, segments, idx+1, newParams, resourcePath); m != nil {
					return m
				}
			}
		}
	}

	// case 3: path param {name}
	if node.paramChild != nil {
		newParams := copyParams(params)
		key := node.paramName
		if key == "" {
			key = "_"
		}
		newParams[key] = seg
		if m := pt.search(node.paramChild, segments, idx+1, newParams, resourcePath); m != nil {
			return m
		}
	}

	// case 4: suffix wildcard
	if node.hasWildcard {
		paramsCopy := copyParams(params)
		return &PathMatch{
			Service:    node.wildcardService,
			Resource:   node.wildcardResource,
			PathParams: paramsCopy,
			Remaining:  strings.Join(segments[idx:], "/"),
		}
	}

	return nil
}

func copyParams(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func matchesPattern(segment, pattern string) bool {
	if !strings.Contains(pattern, "{") {
		return false
	}
	segExt, patExt := "", ""
	if i := strings.LastIndex(segment, "."); i != -1 {
		segExt = segment[i:]
	}
	if i := strings.LastIndex(pattern, "."); i != -1 {
		patExt = pattern[i:]
	}
	return segExt != "" && segExt == patExt
}

var reParam = regexp.MustCompile(`\{([^}]+)\}`)

func extractPatternParamName(pattern string) string {
	m := reParam.FindStringSubmatch(pattern)
	if len(m) > 1 {
		return m[1]
	}
	return "_"
}

func extractParamFromPattern(segment string) string {
	if i := strings.LastIndex(segment, "."); i != -1 {
		return segment[:i]
	}
	return segment
}
