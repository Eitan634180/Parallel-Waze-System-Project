export const sampleRoutePayload = {
    id: 'route-1',
    steps: [
        { node_idx: 10, lat: 32.0000, lon: 34.0000, distance_m: 0, base_time_sec: 0 },
        { node_idx: 11, lat: 32.0000, lon: 34.0010, edge_id: 11, distance_m: 100, base_time_sec: 10 },
        { node_idx: 12, lat: 32.0010, lon: 34.0010, edge_id: 12, distance_m: 220, base_time_sec: 25 },
    ],
    total_dist_m: 220,
    total_time_sec: 25,
    congestion_ahead: false,
    congested_edges: 0,
};
