import { searchLocationByName } from '../../services/rest/navigation-api.js';
import { APP_TIMINGS } from '../../app/app-config.js';
import { SEARCH_RESULTS_UI } from './config.js';

export function setupSearchInput(inputId, suggestionsId, onSelect, onInvalidate = null) {
    const input = document.getElementById(inputId);
    const dropdown = document.getElementById(suggestionsId);
    let searchTimeout = null;
    let requestSeq = 0;

    input.addEventListener('input', () => {
        clearTimeout(searchTimeout);
        const seq = ++requestSeq;
        if (onInvalidate) {
            onInvalidate();
        }
        searchTimeout = setTimeout(async () => {
            const query = input.value.trim();
            if (!query) {
                dropdown.innerHTML = '';
                dropdown.classList.remove('active');
                return;
            }

            const results = await searchLocationByName(query);
            if (seq !== requestSeq || input.value.trim() !== query) {
                return;
            }

            dropdown.innerHTML = '';
            if (results.length > 0) {
                dropdown.classList.add('active');
                results.forEach((r, idx) => {
                    const div = document.createElement('div');
                    div.className = 'suggestion-item';
                    const span = document.createElement('span');
                    span.textContent = r.displayName;
                    div.appendChild(span);
                    div.style.animationDelay = `${idx * SEARCH_RESULTS_UI.animationStaggerSec}s`;
                    div.addEventListener('click', () => {
                        input.value = r.displayName;
                        dropdown.classList.remove('active');
                        onSelect({ lat: r.lat, lng: r.lng, name: r.displayName });
                    });
                    dropdown.appendChild(div);
                });
            } else {
                dropdown.classList.remove('active');
            }
        }, APP_TIMINGS.searchDebounceMs);
    });

    document.addEventListener('click', (e) => {
        if (!e.target.closest(`#${inputId}`) && !e.target.closest(`#${suggestionsId}`)) {
            dropdown.classList.remove('active');
        }
    });
}
