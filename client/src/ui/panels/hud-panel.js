import { formatDuration, getArrivalTime } from '../../utils/formatters.js';
import { CSS_CLASSES, DOM_IDS, PANEL_TEXT, TRAFFIC_LEVELS } from '../ui-constants.js';
import { getTurnInfo } from '../../features/driving/traffic-model.js';

const DISPLAY_STYLES = {
    flex: 'flex',
    none: 'none',
};

const MATH_CONSTANTS = {
    metersPerKilometer: 1000,
    zeroDirection: 0,
};

const FORMAT_CONFIG = {
    distanceDecimals: 1,
    turnDistanceDecimals: 0,
};

const TURN_ANGLES = {
    straight: 20,
    slight: 45,
    regular: 120,
    sharp: 160,
};

const DIRECTIONS = {
    left: 'left',
    right: 'right',
};

const TURN_ICONS = {
    destination: '🏁',
    straight: '↑',
    slightLeft: '↖',
    slightRight: '↗',
    left: '←',
    right: '→',
    sharpLeft: '↙',
    sharpRight: '↘',
    uTurn: '⤺',
};

const TURN_MESSAGES = {
    destination: 'Arrive at destination',
    straight: 'Continue straight',
    slight: (side) => `Slight ${side} turn`,
    turn: (side) => `Turn ${side}`,
    sharp: (side) => `Sharp ${side} turn`,
    uTurn: 'U-turn',
};

export function updateETA(seconds) {
    const durationStr = formatDuration(seconds);
    document.getElementById(DOM_IDS.etaValue).textContent = durationStr;
    document.getElementById(DOM_IDS.hudEta).textContent = durationStr;
    document.getElementById(DOM_IDS.hudArrivalTime).textContent = getArrivalTime(seconds);
}

export function updateDistance(meters, elementId = DOM_IDS.distanceValue) {
    const km = (meters / MATH_CONSTANTS.metersPerKilometer).toFixed(FORMAT_CONFIG.distanceDecimals);
    document.getElementById(elementId).textContent = `${km} km`;
}

export function updateTrafficStatus(status = TRAFFIC_LEVELS.normal, detail = '') {
    const el = document.getElementById(DOM_IDS.trafficStatus);
    if (!el) return;

    if (typeof status === 'boolean') status = status ? TRAFFIC_LEVELS.congested : TRAFFIC_LEVELS.normal;

    if (status === TRAFFIC_LEVELS.heavy) {
        el.textContent = PANEL_TEXT.heavy;
        el.className = 'stat-value heavy';
    } else if (status === TRAFFIC_LEVELS.congested) {
        el.textContent = PANEL_TEXT.congested;
        el.className = 'stat-value congested';
    } else {
        el.textContent = PANEL_TEXT.normal;
        el.className = 'stat-value';
    }
    el.title = detail || '';
}

export function toggleDrivingHUD(isDriving) {
    const hud = document.getElementById(DOM_IDS.drivingHud);
    const sidebar = document.getElementById(DOM_IDS.sidebar);
    const stopBtn = document.getElementById(DOM_IDS.stopDriveButton);
    const startBtn = document.getElementById(DOM_IDS.startDriveButton);

    if (isDriving) {
        hud.classList.remove(CSS_CLASSES.hidden);
        sidebar.classList.add(CSS_CLASSES.hidden);
        stopBtn.style.display = DISPLAY_STYLES.flex;
        startBtn.style.display = DISPLAY_STYLES.none;
    } else {
        hud.classList.add(CSS_CLASSES.hidden);
        sidebar.classList.remove(CSS_CLASSES.hidden);
        stopBtn.style.display = DISPLAY_STYLES.none;
        startBtn.style.display = '';
    }
}

export function updateTurnInfo(distanceLeftOnStep, currentStep, nextStep) {
    const distEl = document.getElementById(DOM_IDS.turnDistance);
    const actionEl = document.getElementById(DOM_IDS.turnAction);
    const iconEl = document.getElementById(DOM_IDS.turnIcon);

    if (distEl) {
        distEl.textContent = `${distanceLeftOnStep.toFixed(FORMAT_CONFIG.turnDistanceDecimals)} m`;
    }

    if (!currentStep) return;

    if (!nextStep) {
        if (actionEl) actionEl.textContent = TURN_MESSAGES.destination;
        if (iconEl) iconEl.textContent = TURN_ICONS.destination;
        return;
    }

    const { angle, direction } = getTurnInfo(currentStep, nextStep);
    const instruction = getTurnInstruction(angle, direction);

    if (actionEl) actionEl.textContent = instruction.text;
    if (iconEl) iconEl.textContent = instruction.icon;
}

function getTurnInstruction(angle, direction) {
    const side = direction > MATH_CONSTANTS.zeroDirection ? DIRECTIONS.left : DIRECTIONS.right;

    if (angle < TURN_ANGLES.straight) {
        return { text: TURN_MESSAGES.straight, icon: TURN_ICONS.straight };
    } else if (angle < TURN_ANGLES.slight) {
        return {
            text: TURN_MESSAGES.slight(side),
            icon: side === DIRECTIONS.left ? TURN_ICONS.slightLeft : TURN_ICONS.slightRight
        };
    } else if (angle < TURN_ANGLES.regular) {
        return {
            text: TURN_MESSAGES.turn(side),
            icon: side === DIRECTIONS.left ? TURN_ICONS.left : TURN_ICONS.right
        };
    } else if (angle < TURN_ANGLES.sharp) {
        return {
            text: TURN_MESSAGES.sharp(side),
            icon: side === DIRECTIONS.left ? TURN_ICONS.sharpLeft : TURN_ICONS.sharpRight
        };
    } else {
        return { text: TURN_MESSAGES.uTurn, icon: TURN_ICONS.uTurn };
    }
}