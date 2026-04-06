import { MAP_COLORS, MAP_LAYER_STYLE } from '../map-config.js';

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
            color: MAP_COLORS.routeInspector,
            weight: MAP_LAYER_STYLE.debugRouteWeight,
            opacity: MAP_LAYER_STYLE.debugRouteOpacity,
            lineCap: 'round',
        });

        line.on('click', () => {
            edgeLines.forEach((edgeLine) => edgeLine.setStyle({ color: MAP_COLORS.routeInspector, weight: MAP_LAYER_STYLE.debugRouteWeight }));
            line.setStyle({ color: MAP_COLORS.routeInspectorSelected, weight: MAP_LAYER_STYLE.debugRouteWeightSelected });
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
            radius: index === 0 || index === routeObj.steps.length - 1 ? MAP_LAYER_STYLE.inspectorTerminalNodeRadius : MAP_LAYER_STYLE.inspectorNodeRadius,
            fillColor: index === 0 ? MAP_COLORS.primaryRoute : index === routeObj.steps.length - 1 ? MAP_COLORS.destination : MAP_COLORS.waypoint,
            fillOpacity: 1,
            color: MAP_COLORS.endpointText,
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
