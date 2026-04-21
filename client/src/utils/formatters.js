import {
    MILLISECONDS_PER_SECOND,
    SECONDS_PER_HOUR,
    SECONDS_PER_MINUTE,
} from './time.js';

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
    const hrs = Math.floor(seconds / SECONDS_PER_HOUR);
    const mins = Math.floor((seconds % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE);
    if (hrs > 0) return `${hrs}${DISPLAY.hourSuffix} ${mins}${DISPLAY.minuteSuffix}`;
    return `${mins}${DISPLAY.minutesLabel}`;
}

export function getArrivalTime(secondsFromNow) {
    const arrivalDate = new Date(Date.now() + secondsFromNow * MILLISECONDS_PER_SECOND);
    const hours = arrivalDate.getHours().toString().padStart(DISPLAY.padLength, DISPLAY.padCharacter);
    const mins = arrivalDate.getMinutes().toString().padStart(DISPLAY.padLength, DISPLAY.padCharacter);
    return `${hours}${DISPLAY.timeSeparator}${mins}`;
}
