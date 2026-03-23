export function formatDuration(seconds) {
    if (!seconds || seconds < 0) return '--';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (hrs > 0) return `${hrs}h ${mins}m`;
    return `${mins} min`;
}

export function getArrivalTime(secondsFromNow) {
    const arrivalDate = new Date(Date.now() + secondsFromNow * 1000);
    const hours = arrivalDate.getHours().toString().padStart(2, '0');
    const mins = arrivalDate.getMinutes().toString().padStart(2, '0');
    return `${hours}:${mins}`;
}
