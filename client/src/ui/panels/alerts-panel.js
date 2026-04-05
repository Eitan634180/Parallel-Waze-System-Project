import { formatDuration } from '../../utils/formatters.js';
import { CSS_CLASSES, DOM_IDS, PANEL_TEXT, UI_TIMINGS } from '../ui-constants.js';

const ANIMATIONS = {
    toastIn: 'toastIn 0.3s ease forwards',
    toastOut: 'toastOut 0.3s ease forwards',
};

const DISTANCE_CONVERSION = {
    metersPerKilometer: 1000,
};

export function showAlert(title, message) {
    const toast = document.getElementById(DOM_IDS.alertToast);
    document.getElementById(DOM_IDS.alertTitle).textContent = title;
    document.getElementById(DOM_IDS.alertMessage).textContent = message;
    toast.classList.remove(CSS_CLASSES.hidden);
    toast.style.animation = ANIMATIONS.toastIn;

    setTimeout(() => {
        toast.style.animation = ANIMATIONS.toastOut;
        setTimeout(() => toast.classList.add(CSS_CLASSES.hidden), UI_TIMINGS.alertHideAnimationBufferMs);
    }, UI_TIMINGS.alertHideDelayMs);
}

export function onArrival(distance, eta) {
    document.getElementById(DOM_IDS.arrivalOverlay)?.remove();

    const overlay = document.createElement('div');
    overlay.id = DOM_IDS.arrivalOverlay;
    overlay.innerHTML = `
    <div class="arrival-card">
            <div class="arrival-icon">&#x1F6A9;</div>
            <h2>${PANEL_TEXT.arrivalTitle}</h2>
            <p>${PANEL_TEXT.arrivalDescription}</p>
            <div class="arrival-stats">
                <div class="a-stat">
                    <span class="a-label">Total Distance</span>
                    <span class="a-val">${(distance / DISTANCE_CONVERSION.metersPerKilometer).toFixed(1)} km</span>
                </div>
                <div class="a-stat">
                    <span class="a-label">Driving Time</span>
                    <span class="a-val">${formatDuration(eta)}</span>
                </div>
            </div>
            <button type="button" id="${DOM_IDS.arrivalCloseButton}">${PANEL_TEXT.arrivalButton}</button>
        </div>
    `;
    document.body.appendChild(overlay);
    document.getElementById(DOM_IDS.arrivalCloseButton)?.addEventListener('click', () => {
        overlay.remove();
    });
}

export function toggleLoadingState(isLoading) {
    document.getElementById(DOM_IDS.navigateButton).disabled = isLoading;
    if (isLoading) document.body.classList.add(CSS_CLASSES.loading);
    else document.body.classList.remove(CSS_CLASSES.loading);
}
