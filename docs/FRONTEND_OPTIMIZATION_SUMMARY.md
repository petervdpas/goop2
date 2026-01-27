# Frontend JavaScript & CSS Optimization Summary

## Created New Utility Files

### 1. `/frontend/src/utils.js` ✅ CREATED
Extracted duplicate DOM and utility functions from main.js:
- `clear()`, `el()`, `div()`, `btn()`, `input()`, `h1()`, `h2()`, `p()`
- `normalizeTheme()`, `applyTheme()`, `normalizeBase()`

**Impact**: Eliminates ~70 lines of duplicate code, makes functions reusable

### 2. `/internal/ui/assets/js/99-dialogs.js` ✅ CREATED  
Centralized dialog utilities for editor and other UI components:
- `dlgAsk()` - Input dialogs with validation
- `dlgAlert()` - Simple alert dialogs
- `dlgBase()` - Shared dialog construction logic

**Impact**: Reduces ~100 lines of dialog creation code in editor.js

## Recommended Refactorings

### JavaScript Optimizations

#### 1. Update `/frontend/src/main.js` ✅ DONE
- Import functions from `utils.js` instead of defining locally
- Removed duplicate helper functions

#### 2. Update `/internal/ui/assets/js/50-editor.js`
Replace dialog functions with:
```javascript
const { dlgAsk, dlgAlert } = window.Goop.dialogs;
```
Remove ~150 lines of duplicate dialog code

#### 3. Consolidate theme normalization
Current duplicates in:
- `/frontend/src/main.js` (now uses utils.js)
- `/internal/ui/assets/js/20-theme.js` (uses own `normalize()`)
- `/app.go` (Go version)

**Recommendation**: Use utils.js version in frontend, keep separate Go implementation

### CSS Optimizations

#### 1. **Duplicate Theme Variables**
Files with identical CSS variable definitions:
- `/frontend/src/style.css` (lines 5-58)
- `/internal/ui/assets/css/00-vars.css` (exact duplicate)

**Fix**: Create shared `_variables.css` and import in both locations
```css
/* shared/_variables.css */
@import url('_variables.css');
```

#### 2. **Duplicate Background Gradients**
Same radial-gradient pattern in:
- `/frontend/src/style.css` (lines 76-78)
- `/internal/ui/assets/css/10-base.css` (lines 14-16)

**Fix**: Define as CSS variable:
```css
:root {
  --bg-pattern: radial-gradient(1200px 700px at 18% -10%, var(--bg-grad-1), var(--bg-grad-3) 62%),
                radial-gradient(900px 600px at 92% 18%, var(--bg-grad-2), var(--bg-grad-3) 58%),
                linear-gradient(180deg, rgba(0,0,0,0.10), rgba(0,0,0,0.0) 30%);
}

body {
  background: var(--bg-pattern), var(--bg);
}
```

#### 3. **Duplicate Base Styles**
Repeated reset/base styles in both frontend and internal CSS:
- `box-sizing`, `html/body height`, font-family, etc.

**Fix**: Extract to shared `_base.css`

#### 4. **Duplicate Utility Classes**
Classes like `.hidden`, `.muted`, `.small` defined in multiple places

**Fix**: Create `_utilities.css` with all helper classes

### LocalStorage Wrappers

Already optimized! ✅
- `safeLocalStorageGet()` and `safeLocalStorageSet()` in `/internal/ui/assets/js/00-core.js`
- Used consistently across codebase

## File Size Impact (Estimated)

| File | Before | After | Savings |
|------|--------|-------|---------|
| main.js | ~378 lines | ~310 lines | ~18% |
| 50-editor.js | ~407 lines | ~280 lines | ~31% |
| style.css | ~427 lines | ~350 lines | ~18% |
| **Total** | **1212 lines** | **940 lines** | **~22% reduction** |

## Additional Recommendations

### 1. Minification & Bundling
Current setup doesn't minify internal UI assets. Consider:
```bash
# Add to build process
terser internal/ui/assets/js/*.js --compress --mangle -o dist/app.min.js
cssnano internal/ui/assets/css/*.css -o dist/app.min.css
```

### 2. CSS Organization
Current structure is good (numbered prefix system), but consider:
- Combining 00-vars + 10-base into `_foundation.css`
- Using CSS `@layer` for better specificity control

### 3. JavaScript Module System
Internal UI scripts use IIFE pattern (good!), but could benefit from:
- ES6 modules for better tree-shaking
- Build step to bundle for production

### 4. Remove Unused CSS
Run PurgeCSS to remove unused selectors:
```bash
purgecss --css internal/ui/assets/css/*.css --content internal/ui/templates/*.html
```

### 5. Duplicate Dialog Patterns
The `q()` function in editor.js is duplicated. Now in `99-dialogs.js` as `createElement()`

## Migration Steps

1. ✅ Create utility files (Done)
2. ✅ Update main.js imports (Done)
3. Add script tag for `99-dialogs.js` in layout.html
4. Update editor.js to use `Goop.dialogs`
5. Create shared CSS variables file
6. Update both style.css files to import shared variables
7. Test thoroughly in both launcher and viewer UIs

## Testing Checklist

- [ ] Launcher theme toggle works
- [ ] Peer creation dialog works
- [ ] Viewer theme sync with bridge works
- [ ] Editor dialogs (new, rename, delete) work
- [ ] All CSS themes (light/dark) render correctly
- [ ] No console errors in browser
- [ ] Build process completes without errors

## Browser Compatibility

All optimizations maintain compatibility with:
- Chrome/Edge 90+
- Firefox 88+
- Safari 14+

Using standard ES6+ features (modules, async/await, arrow functions).
