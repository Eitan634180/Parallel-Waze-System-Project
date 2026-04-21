export const DRIVING_PHYSICS = {
  followingTimeSec: 1.8,
  minGapM: 7,
  maxTrackedGapM: 80,
  routeCaptureM: 18,
  minStepDistanceM: 0.1,
  offRouteDriftPerMs: 0.0000006,
  sharpTurnThresholdDeg: 25,
};

export const DRIVING_SPEED = {
  minTargetSpeedKmh: 0.5,
  minTargetSpeedCapKmh: 8,
  targetSpeedBufferRatio: 1.05,
  defaultAccelerationMps2: 2.0,
  defaultBrakingMps2: 3.0,
  turnSpeedCapKmh: 20,
};

export const DRIVER_PROFILE_RANGES = {
  paceBias: [0.85, 1.15],
  accelerationMps2: [1.2, 2.8],
  brakingMps2: [1.8, 3.6],
  junctionBias: [0.85, 1.3],
};

export const INTERSECTION_DELAYS = {
  minDelayMs: 250,
  sharpTurnAngleDeg: 120,
  turnAngleDeg: 65,
  lowSpeedRoadThresholdKmh: 35,
  shortSegmentLengthM: 35,
  sharpTurnDelayMs: 2200,
  turnDelayMs: 1200,
  lowSpeedRoadDelayMs: 550,
  shortSegmentDelayMs: 300,
};

export const TRAFFIC_EVALUATION = {
  aheadWindowSize: 6,
  heavySlowEdgeCount: 2,
  heavyWorstRatio: 0.6,
  slowdownRatio: 0.95,
};
