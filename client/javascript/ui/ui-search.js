import { searchLocationByName } from '../api/api-rest.js';
import { SEARCH_DELAY } from '../core/config.js';

export function setupSearchInput(inputId, suggestionsId, onSelect) {
    const input = document.getElementById(inputId);
    const dropdown = document.getElementById(suggestionsId);
    let searchTimeout = null;

    input.addEventListener('input', () => {
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(async () => {
            const results = await searchLocationByName(input.value);
            dropdown.innerHTML = '';
            if (results.length > 0) {
                dropdown.classList.add('active');
                results.forEach((r, idx) => {
                    const div = document.createElement('div');
                    div.className = 'suggestion-item';
                    const span = document.createElement('span');
                    span.textContent = r.displayName;
                    div.appendChild(span);
                    div.style.animationDelay = `${idx * 0.05}s`;
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
        }, SEARCH_DELAY);
    });

    document.addEventListener('click', (e) => {
        if (!e.target.closest(`#${inputId}`) && !e.target.closest(`#${suggestionsId}`)) {
            dropdown.classList.remove('active');
        }
    });
}