let tickMs = 100;
let timer = null;

function start() {
    stop();
    timer = setInterval(() => {
        postMessage({ type: 'tick', now: performance.now() });
    }, tickMs);
}

function stop() {
    if (!timer) return;
    clearInterval(timer);
    timer = null;
}

onmessage = (event) => {
    if (event.data?.type === 'start') {
        tickMs = event.data.tickMs || tickMs;
        start();
    } else if (event.data?.type === 'stop') {
        stop();
    }
};
