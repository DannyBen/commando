const search = document.querySelector('#feature-search');
const featureList = document.querySelector('#feature-list');
const count = document.querySelector('#result-count');
const empty = document.querySelector('#empty-state');
const themeToggle = document.querySelector('#theme-toggle');
const systemTheme = matchMedia('(prefers-color-scheme: dark)');
let themeOverride = null;

for (const group of window.commandoFeatures) {
  for (const feature of group.items) {
    const row = document.createElement('li');
    row.className = 'feature-row';
    row.dataset.search = `${feature.id} ${feature.search || ''}`;
    row.dataset[feature.kind === 'setting' ? 'settingId' : 'commandId'] = feature.id;

    const main = document.createElement('div');
    main.className = 'feature-main';
    const category = document.createElement('span');
    category.className = 'category';
    category.textContent = group.category;

    const body = document.createElement('div');
    body.className = 'feature-text';
    const title = document.createElement('div');
    title.className = 'feature-title';
    const name = document.createElement('strong');
    feature.title.split(' › ').forEach((part, index) => {
      if (index) {
        const chevron = document.createElement('span');
        chevron.className = 'chevron';
        chevron.textContent = '›';
        name.append(' ', chevron, ' ');
      }
      name.append(part);
    });

    const access = document.createElement('span');
    access.className = feature.keys ? 'keys' : 'search-cue';
    if (feature.keys) {
      feature.keys.forEach((key, index) => {
        if (index) access.append('/');
        const badge = document.createElement('kbd');
        badge.textContent = key;
        access.append(badge);
      });
    } else {
      if (feature.kind === 'setting') {
        const type = document.createElement('span');
        type.className = 'feature-type';
        type.textContent = 'Setting';
        access.append(type, ' · ');
      } else {
        access.append('search ');
      }
      const query = document.createElement('code');
      query.textContent = feature.query;
      access.append(query);
    }

    const description = document.createElement('p');
    description.textContent = feature.description;
    title.append(name, access);
    body.append(title, description);
    main.append(category, body);
    row.append(main);
    featureList.append(row);
  }
}
const rows = [...featureList.children];

try {
  const saved = localStorage.getItem('commando-site-theme');
  if (saved === 'light' || saved === 'dark') themeOverride = saved;
} catch { /* Local files can deny storage; the toggle still works for this visit. */ }

function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  document.querySelector('#brand-logo').src = `assets/logo-${theme}.svg`;
  document.querySelector('#overview-image').src = `assets/overview-${theme}.png`;
  document.querySelector('meta[name="theme-color"]').content = theme === 'dark' ? '#0b0d12' : '#f5f3ef';
  themeToggle.querySelector('.theme-icon').textContent = theme === 'dark' ? '☾' : '☀';
  themeToggle.querySelector('.theme-name').textContent = `${theme === 'dark' ? 'Dark' : 'Light'} theme`;
  themeToggle.setAttribute('aria-label', `Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`);

  for (const video of document.querySelectorAll('video[data-light-src]')) {
    video.poster = video.dataset[`${theme}Poster`];
    const source = video.dataset[`${theme}Src`];
    if (!video.getAttribute('src')?.endsWith(source)) video.src = source;
    video.play().catch(() => {});
  }
}

applyTheme(themeOverride || (systemTheme.matches ? 'dark' : 'light'));

themeToggle.addEventListener('click', () => {
  themeOverride = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
  try { localStorage.setItem('commando-site-theme', themeOverride); } catch { /* Keep the current-page choice. */ }
  applyTheme(themeOverride);
});

systemTheme.addEventListener('change', event => {
  if (!themeOverride) applyTheme(event.matches ? 'dark' : 'light');
});

const featurePalette = document.querySelector('.feature-palette');
let themeFadeFrame = 0;

function updateThemeToggleVisibility() {
  themeFadeFrame = 0;
  const distance = featurePalette.getBoundingClientRect().top;
  const opacity = Math.max(0, Math.min(1, (distance - 64) / 240));
  themeToggle.style.opacity = opacity;
  themeToggle.inert = opacity === 0;
}

function queueThemeToggleVisibility() {
  if (!themeFadeFrame) themeFadeFrame = requestAnimationFrame(updateThemeToggleVisibility);
}

window.addEventListener('scroll', queueThemeToggleVisibility, {passive: true});
window.addEventListener('resize', queueThemeToggleVisibility);
updateThemeToggleVisibility();

const latestZipCacheKey = 'commando-site-latest-zip';
const latestZipCacheAge = 60 * 60 * 1000;

function showLatestZip(url, version) {
  if (typeof url !== 'string' || typeof version !== 'string' ||
      !/^https:\/\/github\.com\/DannyBen\/commando\/releases\/download\/[^/]+\/commando-.+-windows_amd64\.zip$/.test(url)) return false;
  document.querySelector('#download-link').href = url;
  document.querySelector('#footer-download-link').href = url;
  document.querySelector('#download-note').textContent = `${version.replace(/^v/i, 'v')} (ZIP)`;
  return true;
}

async function linkLatestZip() {
  try {
    const cached = JSON.parse(localStorage.getItem(latestZipCacheKey));
    if (cached && Date.now() - cached.checkedAt < latestZipCacheAge && showLatestZip(cached.url, cached.version)) return;
  } catch { /* Continue without a browser cache. */ }

  try {
    const response = await fetch('https://api.github.com/repos/DannyBen/commando/releases/latest');
    if (!response.ok) return;
    const release = await response.json();
    const asset = release.assets?.find(item => /^commando-.+-windows_amd64\.zip$/.test(item.name));
    if (showLatestZip(asset?.browser_download_url, release.tag_name)) {
      try {
        localStorage.setItem(latestZipCacheKey, JSON.stringify({url: asset.browser_download_url, version: release.tag_name, checkedAt: Date.now()}));
      } catch { /* The resolved link still works for this visit. */ }
    }
  } catch { /* The releases page remains available as the fallback link. */ }
}

linkLatestZip();

function filterFeatures() {
  const terms = search.value.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  let visible = 0;

  for (const row of rows) {
    const text = `${row.textContent} ${row.dataset.search || ''}`.toLocaleLowerCase();
    const words = text.match(/[\p{L}\p{N}]+/gu) || [];
    row.hidden = !terms.every(term => /^[\p{L}\p{N}]+$/u.test(term)
      ? words.some(word => word.startsWith(term))
      : text.includes(term));
    if (!row.hidden) visible++;
  }

  count.textContent = `${visible} shown`;
  empty.hidden = visible !== 0;
  if (terms.length) {
    const palette = document.querySelector('.feature-palette');
    window.scrollTo({top: window.scrollY + palette.getBoundingClientRect().top, behavior: 'instant'});
  }
}

search.addEventListener('input', filterFeatures);
filterFeatures();

document.addEventListener('keydown', event => {
  if (event.key === '/' && !event.ctrlKey && !event.altKey && !event.metaKey &&
      !event.target.closest('input, textarea, [contenteditable]')) {
    event.preventDefault();
    search.focus();
  }

  if (event.key === 'Escape' && document.activeElement === search && search.value) {
    search.value = '';
    filterFeatures();
  }
});
