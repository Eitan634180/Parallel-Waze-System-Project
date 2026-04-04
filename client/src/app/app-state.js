const defaultDriveState = {
    isActive: false,
    sessionId: null,
    carPos: null,
    distanceLeft: 0,
    totalDistanceDrivenM: 0,
    stepProgress: 0,
    currentRoadIndex: 0,
    offRouteOffset: [0, 0],
    isDrifting: false,
    startTimeMs: null,
};

const defaultSimState = {
    recommendedSpeeds: new Map(),
    pendingEdgeEvents: [],
    currentEdgeTimeMs: 0,
    driverProfile: null,
    motionState: null,
    cars: [],
};

export const state = {
    routing: {
        source: null,
        dest: null,
        allRoutes: [],
        currentIndex: 0,
        activeObj: null,
        activeLegs: [],
    },
    drive: { ...defaultDriveState },
    sim: { ...defaultSimState },
    debug: {
        inspectorEnabled: false,
        carsVisible: true,
        serverData: null,
    }
};

export function resetDrivingSession() {
    state.drive = {
        isActive: false,
        sessionId: null,
        carPos: null,
        distanceLeft: 0,
        totalDistanceDrivenM: 0,
        stepProgress: 0,
        currentRoadIndex: 0,
        offRouteOffset: [0, 0],
        isDrifting: false,
        startTimeMs: null,
    };
    state.sim = {
        recommendedSpeeds: new Map(),
        pendingEdgeEvents: [],
        currentEdgeTimeMs: 0,
        driverProfile: null,
        motionState: null,
        cars: [],
    };
    state.debug.serverData = null;
}
