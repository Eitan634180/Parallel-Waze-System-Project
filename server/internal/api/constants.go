package api

const (
	searchRoutePath           = "/search"
	routeRoutePath            = "/route"
	sessionRoutePath          = "/session"
	sessionSubtreeRoutePath   = "/session/"
	simulationRoutePath       = "/simulation"
	simulationRandomRoutePath = "/simulation/random"
	simulationWSRoutePath     = "/simulation/ws"

	headerOrigin                 = "Origin"
	headerVary                   = "Vary"
	headerContentType            = "Content-Type"
	headerAllowOrigin            = "Access-Control-Allow-Origin"
	headerAllowMethods           = "Access-Control-Allow-Methods"
	headerAllowHeaders           = "Access-Control-Allow-Headers"
	headerAcceptLanguage         = "Accept-Language"
	corsAllowedMethods           = "GET,POST,DELETE,OPTIONS"
	corsAllowedHeaders           = headerContentType
	jsonContentType              = "application/json"
	originNotAllowedMessage      = "origin not allowed"
	sessionPathPrefix            = sessionSubtreeRoutePath
	sessionPathSeparator         = "/"
	sessionPathSplitLimit        = 2
	searchUpstreamUnavailableMsg = "search upstream unavailable"
	searchUpstreamFailedMsg      = "search upstream failed"
	searchDecodeFailedMsg        = "failed to decode search results"
	searchRequestFailedMsg       = "failed to create search request"

	wsMessageTypePing        = "ping"
	wsMessageTypeETAUpdate   = "eta_update"
	wsMessageTypeReroute     = "reroute"
	wsMessageTypeSpeedUpdate = "speed_update"
	wsMessageTypeDebugUpdate = "debug_update"
	wsMessageTypeSnapshot    = "snapshot"
)

