// Default device name for pairing, derived from the browser's user-agent family.
// Contract: `defaultDeviceName(userAgent)` returns a short human label such as
// "iPhone (Safari)" or "Android phone (Chrome)"; it never includes version
// numbers or anything private, and falls back to "Phone" for unknown agents.

/**
 * @param userAgent The `navigator.userAgent` string.
 * @returns A short label naming the device family and browser.
 */
export function defaultDeviceName(userAgent: string): string {
  const ua = userAgent.toLowerCase();
  let device = 'Phone';
  if (ua.includes('ipad')) device = 'iPad';
  else if (ua.includes('iphone')) device = 'iPhone';
  else if (ua.includes('android')) device = ua.includes('mobile') ? 'Android phone' : 'Android tablet';
  else if (ua.includes('windows')) device = 'Windows PC';
  else if (ua.includes('mac os') || ua.includes('macintosh')) device = 'Mac';
  else if (ua.includes('cros')) device = 'Chromebook';
  else if (ua.includes('linux')) device = 'Linux PC';

  let browser = '';
  if (ua.includes('firefox') || ua.includes('fxios')) browser = 'Firefox';
  else if (ua.includes('edg/') || ua.includes('edgios')) browser = 'Edge';
  else if (ua.includes('samsungbrowser')) browser = 'Samsung Internet';
  else if (ua.includes('opr/') || ua.includes('opera')) browser = 'Opera';
  else if (ua.includes('chrome') || ua.includes('crios')) browser = 'Chrome';
  else if (ua.includes('safari')) browser = 'Safari';

  return browser ? `${device} (${browser})` : device;
}
