import { formatDuration, getArrivalTime } from '../utils/utils.js';

export function renderRouteOptions(routes, onSelectRoute) {
    const container = document.getElementById('route-options-container');
    container.innerHTML = '';

    routes.forEach((route, idx) => {
        const isFastest = idx === 0;
        const div = document.createElement('div');
        div.className = `route-option-card ${idx === 0 ? 'active' : ''}`;
        div.id = `route-option-${idx}`;

        const distKm = (route.distance / 1000).toFixed(1);
        const duration = formatDuration(route.dynamicETA);
        const arrivalTime = getArrivalTime(route.dynamicETA);

        div.innerHTML = `
            <div class="route-option-main">
                ${duration}
                ${isFastest ? '<span class="route-option-tag">Fastest</span>' : ''}
            </div>
            <div class="route-option-sub">
                <span>${distKm} km</span>
                <span class="route-sep">|</span>
                <span>Arrives ${arrivalTime}</span>
            </div>
        `;

        div.addEventListener('click', () => onSelectRoute(idx));
        container.appendChild(div);
    });
}