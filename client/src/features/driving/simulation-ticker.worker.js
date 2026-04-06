let tickMs = 100;
let timer = null;

const MESSAGE_TYPES = {
    start: 'start',
    stop: 'stop',
    tick: 'tick',
};

function start() {
    stop();
    timer = setInterval(() => {
        postMessage({ type: MESSAGE_TYPES.tick, now: performance.now() });
    }, tickMs);
}

function stop() {
    if (!timer) return;
    clearInterval(timer);
    timer = null;
}

onmessage = (event) => {
    if (event.data?.type === MESSAGE_TYPES.start) {
        tickMs = event.data.tickMs || tickMs;
        start();
    } else if (event.data?.type === MESSAGE_TYPES.stop) {
        stop();
    }
};
