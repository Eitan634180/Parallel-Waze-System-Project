import { formatDuration } from '../../utils/formatters.js';

export function showAlert(title, message) {
    const toast = document.getElementById('alert-toast');
    document.getElementById('alert-title').textContent = title;
    document.getElementById('alert-message').textContent = message;
    toast.classList.remove('hidden');
    toast.style.animation = 'toastIn 0.3s ease forwards';

    setTimeout(() => {
        toast.style.animation = 'toastOut 0.3s ease forwards';
        setTimeout(() => toast.classList.add('hidden'), 400);
    }, 5000);
}

export function onArrival(distance, eta) {
    document.getElementById('arrival-overlay')?.remove();

    const overlay = document.createElement('div');
    overlay.id = 'arrival-overlay';
    overlay.innerHTML = `
    <div class="arrival-card">
            <div class="arrival-icon">&#x1F6A9;</div>
            <h2>Arrived</h2>
            <p>You have reached your destination</p>
            <div class="arrival-stats">
                <div class="a-stat">
                    <span class="a-label">Total Distance</span>
                    <span class="a-val">${(distance / 1000).toFixed(1)} km</span>
                </div>
                <div class="a-stat">
                    <span class="a-label">Driving Time</span>
                    <span class="a-val">${formatDuration(eta)}</span>
                </div>
            </div>
            <button type="button" id="arrival-close-btn">Close</button>
        </div>
    `;
    document.body.appendChild(overlay);
    document.getElementById('arrival-close-btn')?.addEventListener('click', () => {
        overlay.remove();
    });
}

export function toggleLoadingState(isLoading) {
    document.getElementById('navigate-btn').disabled = isLoading;
    if (isLoading) document.body.classList.add('loading');
    else document.body.classList.remove('loading');
}
