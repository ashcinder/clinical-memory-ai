const tallDesktop = matchMedia('(min-width: 761px) and (hover: hover) and (pointer: fine)');

export function allowViewerPageScroll(canvas, fullscreen, signal) {
 canvas.addEventListener('wheel', event => {
  // Keep ordinary page scrolling available when the viewer extends below the window.
  if (tallDesktop.matches && !fullscreen.open && !event.ctrlKey && !event.metaKey) {
   event.stopImmediatePropagation();
  }
 }, {capture:true, passive:true, signal});
}
