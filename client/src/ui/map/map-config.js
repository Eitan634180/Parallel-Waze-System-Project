export const MAP_DEFAULTS = {
    center: [32.0853, 34.7818],
    debugCarZIndexOffset: 600,
    fitBoundsPadding: [100, 100],
    initialBBoxInsetFraction: 0.3,
    maxBBoxInsetFraction: 0.49,
    maxZoom: 19,
    zoom: 12,
};

export const MAP_TILES = {
    attribution: '&copy; <a href="https://carto.com/">CARTO</a>',
    subdomains: 'abcd',
    url: 'https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png',
};

export const MAP_COLORS = {
    destination: '#ff3d00',
    destinationWalk: '#ff5252',
    endpointBorder: 'white',
    endpointText: '#0f172a',
    primaryRoute: '#33ccff',
    routeInspector: '#007aff',
    routeInspectorSelected: '#ff3d00',
    secondaryRoute: '#94a3b8',
    waypoint: '#ffffff',
};

export const MAP_LAYER_STYLE = {
    debugCarIconAnchor: [7, 7],
    debugCarIconSize: [14, 14],
    debugRouteOpacity: 0.95,
    debugRouteWeight: 5,
    debugRouteWeightSelected: 7,
    endpointRadius: 12,
    endpointFillOpacity: 1,
    endpointWeight: 4,
    hiddenRouteOpacity: 0,
    inspectorNodeFillOpacity: 1,
    inspectorNodeWeight: 1.5,
    inspectorNodeRadius: 3,
    inspectorTerminalNodeRadius: 5,
    primaryRouteOpacity: 0.9,
    primaryRouteWeight: 10,
    secondaryRouteDashArray: '10 10',
    secondaryRouteOpacity: 0.6,
    secondaryRouteWeight: 6,
    vehicleIconAnchor: [15, 22],
    vehicleIconSize: [30, 45],
    walkRouteDashArray: '5, 8',
    walkRouteOpacity: 0.6,
    walkRouteWeight: 3,
};

