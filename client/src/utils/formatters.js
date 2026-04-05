const TIME = {
    hoursPerDay: 24,
    minutesPerHour: 60,
    secondsPerHour: 3600,
    secondsPerMinute: 60,
    millisecondsPerSecond: 1000,
};

const DISPLAY = {
    emptyDuration: '--',
    hourSuffix: 'h',
    minuteSuffix: 'm',
    minutesLabel: ' min',
    padLength: 2,
    padCharacter: '0',
    timeSeparator: ':',
};

export function formatDuration(seconds) {
    if (!seconds || seconds < 0) return DISPLAY.emptyDuration;
    const hrs = Math.floor(seconds / TIME.secondsPerHour);
    const mins = Math.floor((seconds % TIME.secondsPerHour) / TIME.secondsPerMinute);
    if (hrs > 0) return `${hrs}${DISPLAY.hourSuffix} ${mins}${DISPLAY.minuteSuffix}`;
    return `${mins}${DISPLAY.minutesLabel}`;
}

export function getArrivalTime(secondsFromNow) {
    const arrivalDate = new Date(Date.now() + secondsFromNow * TIME.millisecondsPerSecond);
    const hours = arrivalDate.getHours().toString().padStart(DISPLAY.padLength, DISPLAY.padCharacter);
    const mins = arrivalDate.getMinutes().toString().padStart(DISPLAY.padLength, DISPLAY.padCharacter);
    return `${hours}${DISPLAY.timeSeparator}${mins}`;
}
