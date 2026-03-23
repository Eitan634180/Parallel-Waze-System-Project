export function refreshRouteInspector(manager) {
    clearInspectorLayer(manager);
    manager.routeInspector.edgeLines = [];

    if (!manager.routeInspector.enabled || !manager.routeInspector.routeObj) {
        return;
    }

    const { routeObj } = manager.routeInspector;
    const layers = [];
    const edgeLines = [];

    routeObj.legs.forEach((leg, index) => {
        const fromStep = routeObj.steps[index];
        const toStep = routeObj.steps[index + 1];
        const line = L.polyline([
            [leg.from_node[1], leg.from_node[0]],
            [leg.to_node[1], leg.to_node[0]],
        ], {
            color: '#007aff',
            weight: 5,
            opacity: 0.95,
            lineCap: 'round',
        });

        line.on('click', () => {
            edgeLines.forEach((edgeLine) => edgeLine.setStyle({ color: '#007aff', weight: 5 }));
            line.setStyle({ color: '#ff3d00', weight: 7 });
            manager.routeInspector.onEdgeClick?.({
                index,
                edgeId: leg.edge_id,
                distanceM: leg.base_length,
                durationSec: leg.duration_sec,
                speedLimitKmh: leg.speed_limit,
                fromNodeId: fromStep?.node_id ?? 0,
                toNodeId: toStep?.node_id ?? 0,
                fromLat: fromStep?.lat || leg.from_node[1],
                fromLon: fromStep?.lon || leg.from_node[0],
                toLat: toStep?.lat || leg.to_node[1],
                toLon: toStep?.lon || leg.to_node[0],
            });
        });

        edgeLines.push(line);
        layers.push(line);
    });

    routeObj.steps.forEach((step, index) => {
        layers.push(L.circleMarker([step.lat, step.lon], {
            radius: index === 0 || index === routeObj.steps.length - 1 ? 5 : 3,
            fillColor: index === 0 ? '#33ccff' : index === routeObj.steps.length - 1 ? '#ff3d00' : '#ffffff',
            fillOpacity: 1,
            color: '#0f172a',
            weight: 1.5,
        }));
    });

    manager.routeInspector.edgeLines = edgeLines;
    manager.layers.debugRoute = L.layerGroup(layers).addTo(manager.map);
}

export function clearInspectorLayer(manager) {
    if (!manager.layers.debugRoute) {
        return;
    }

    manager.map.removeLayer(manager.layers.debugRoute);
    manager.layers.debugRoute = null;
}
