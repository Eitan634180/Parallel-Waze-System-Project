import { createSession, deleteSession } from '../../services/rest/navigation-api.js';
import { connectToSession, disconnectSession } from '../../services/ws/socket-client.js';

const SESSION_LOG_MESSAGES = {
    deleteFailed: 'Navigation session cleanup failed',
};

export async function openDrivingSession(routeId, callbacks) {
    const sessionId = await createSession(routeId);
    try {
        await connectToSession(sessionId, callbacks);
        return sessionId;
    } catch (error) {
        await safeDeleteSession(sessionId);
        throw error;
    }
}

export async function closeDrivingSession(sessionId) {
    disconnectSession();
    if (sessionId) {
        await safeDeleteSession(sessionId);
    }
}

async function safeDeleteSession(sessionId) {
    try {
        await deleteSession(sessionId);
    } catch (error) {
        console.error(SESSION_LOG_MESSAGES.deleteFailed, error);
    }
}
