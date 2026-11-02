import {mountAtlas} from './viewer.js';
mountAtlas(document.querySelector('#atlas-root'),new URL('..',location.href).pathname.replace(/\/$/,'')).catch(e=>{document.querySelector('#atlas-root').textContent='图谱载入失败：'+e.message;});
