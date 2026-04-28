import { formatDuration, getArrivalTime } from '../../utils/formatters.js';
import { DOM_IDS, PANEL_NUMBERS, UI_KEYS } from '../ui-constants.js';
import { METERS_PER_KILOMETER } from '../../utils/math.js';

const FASTEST_ROUTE_INDEX = 0;
const FASTEST_TAG_HTML = '<span class="route-option-tag">Fastest</span>';
const ROUTE_SEPARATOR_HTML = '<span class="route-sep">|</span>';
export function renderRouteOptions(routes, onSelectRoute) {
    const container = document.getElementById(DOM_IDS.routeOptionsContainer);
    container.innerHTML = '';

    routes.forEach((route, idx) => {
        const isFastest = idx === FASTEST_ROUTE_INDEX;
        const div = document.createElement('div');
        div.className = `route-option-card ${idx === FASTEST_ROUTE_INDEX ? 'active' : ''}`;
        div.id = `${UI_KEYS.routeOptionIdPrefix}${idx}`;

        const distKm = (route.distance / METERS_PER_KILOMETER).toFixed(PANEL_NUMBERS.routeOptionDistancePrecision);
        const duration = formatDuration(route.dynamicETA);
        const arrivalTime = getArrivalTime(route.dynamicETA);

        div.innerHTML = `
            <div class="route-option-main">
                ${duration}
                ${isFastest ? FASTEST_TAG_HTML : ''}
            </div>
            <div class="route-option-sub">
                <span>${distKm} km</span>
                ${ROUTE_SEPARATOR_HTML}
                <span>Arrives ${arrivalTime}</span>
            </div>
        `;

        div.addEventListener('click', () => onSelectRoute(idx));
        container.appendChild(div);
    });
}
