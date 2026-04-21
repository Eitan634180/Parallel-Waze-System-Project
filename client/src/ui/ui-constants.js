export const CSS_CLASSES = {
    active: 'active',
    hidden: 'hidden',
    loading: 'loading',
};

export const DOM_IDS = {
    alertMessage: 'alert-message',
    alertTitle: 'alert-title',
    alertToast: 'alert-toast',
    arrivalCloseButton: 'arrival-close-btn',
    arrivalOverlay: 'arrival-overlay',
    closeDebug: 'close-debug',
    closeRoute: 'close-route',
    debugAddCarButton: 'debug-add-car-btn',
    debugCarCount: 'debug-car-count',
    debugCarStatus: 'debug-car-status',
    debugClearCarsButton: 'debug-clear-cars-btn',
    debugDriftButton: 'debug-drift-btn',
    debugEdgeInfo: 'debug-edge-info',
    debugMainStatus: 'debug-main-status',
    debugPanel: 'debug-panel',
    debugRandomTrafficButton: 'debug-random-traffic-btn',
    debugToggleButton: 'debug-toggle-btn',
    destInput: 'dest-input',
    destSuggestions: 'dest-suggestions',
    distanceValue: 'distance-value',
    drivingHud: 'driving-hud',
    turnDistance: 'turn-distance',
    turnAction: 'turn-action',
    turnIcon: 'turn-icon',
    etaValue: 'eta-value',
    hudArrivalTime: 'hud-arrival-time',
    hudDistanceLeft: 'hud-distance-left',
    hudDstName: 'hud-dst-name',
    hudEta: 'hud-eta',
    hudSpeed: 'hud-speed',
    hudSrcName: 'hud-src-name',
    navigateButton: 'navigate-btn',
    routeOptionsContainer: 'route-options-container',
    routePanel: 'route-panel',
    searchPanel: 'search-panel',
    sidebar: 'sidebar',
    sourceInput: 'source-input',
    sourceSuggestions: 'source-suggestions',
    startDriveButton: 'start-drive-btn',
    stopDriveButton: 'stop-drive-btn',
    toggleDebugCarsButton: 'toggle-debug-cars-btn',
    toggleRouteInspectorButton: 'toggle-route-inspector-btn',
    trafficStatus: 'traffic-status',
};

export const PANEL_TEXT = {
    activeRouteClear: 'No slowdowns detected on the active route.',
    arrivalButton: 'Close',
    arrivalDescription: 'You have reached your destination',
    arrivalTitle: 'Arrived',
    congested: 'Slower',
    debugCarsFailed: 'Debug cars failed',
    disableRouteInspector: 'Disable Route Inspector',
    inspectRouteEdges: 'Inspect Route Edges',
    edgeInfoHint: 'Click a highlighted route segment to inspect its road data.',
    forceOffRouteActive: 'ACTIVE',
    forceOffRouteInactive: 'OFF',
    heavy: 'Heavy',
    hideTestCars: 'Hide Test Cars',
    mainCarHint: 'Start driving to inspect live navigation state.',
    navigationDisconnectedDescription: 'Live server connection was lost.',
    navigationDisconnectedTitle: 'Navigation disconnected',
    navigationFailedTitle: 'Navigation failed',
    noEdgeSelected: 'No edge selected',
    normal: 'Normal',
    offRouteDescription: 'Vehicle left the expected path.',
    offRouteTitle: 'Off-route reroute',
    randomTrafficFailed: 'Random traffic failed',
    routeCalculationFailed: 'Route calculation failed',
    routeUpdated: 'Route updated for current traffic conditions.',
    showTestCars: 'Show Test Cars',
    simulationFeed: 'Simulation feed',
    trafficRerouteTitle: 'Traffic reroute',
};

export const PANEL_NUMBERS = {
    arrivalDistancePrecision: 1,
    distancePrecisionKm: 1,
    edgeInfoDistancePrecision: 1,
    edgeInfoCoordinatePrecision: 5,
    edgeInfoDurationPrecision: 1,
    hudDistancePrecision: 1,
    hudTurnDistancePrecision: 0,
    offRouteDistancePrecision: 1,
    offRouteThresholdPrecision: 0,
    routeOptionDistancePrecision: 1,
    speedDisplayPrecision: 0,
};

export const HUD_TURN_ANGLES = {
    regular: 120,
    sharp: 160,
    slight: 45,
    straight: 20,
};

export const UI_KEYS = {
    routeOptionIdPrefix: 'route-option-',
};

export const UI_TIMINGS = {
    alertHideDelayMs: 5000,
    alertHideAnimationBufferMs: 400,
};

export const TRAFFIC_LEVELS = {
    congested: 'congested',
    heavy: 'heavy',
    normal: 'normal',
};
