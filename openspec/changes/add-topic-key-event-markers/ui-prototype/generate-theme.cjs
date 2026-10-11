// 仅原型制品生成：从唯一主题源复制变量块，不引入产品依赖/工具链。
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const source=path.resolve(__dirname,'../../../../front/app/assets/css/main.css');
const css=fs.readFileSync(source,'utf8');
function block(selector){
 const begin=css.indexOf(selector+' {');if(begin<0)throw Error('Missing theme selector: '+selector);
 const brace=css.indexOf('{',begin);let depth=0,comment=false;
 for(let i=brace;i<css.length;i++){
  if(comment){if(css[i]==='*'&&css[i+1]==='/'){comment=false;i++;}continue;}
  if(css[i]==='/'&&css[i+1]==='*'){comment=true;i++;continue;}
  if(css[i]==='{')depth++;if(css[i]==='}'&&--depth===0)return css.slice(begin,i+1).replace(/\r\n/g,'\n');
 }
 throw Error('Unclosed theme block: '+selector);
}
const selectors=[':root','[data-theme="editorial"]','[data-theme="dark"]'];
const content='/* Generated from front/app/assets/css/main.css; do not hand-pick a new palette. */\n'+selectors.map(block).join('\n\n')+'\n';
fs.writeFileSync(path.join(__dirname,'theme-tokens.css'),content);
fs.writeFileSync(path.join(__dirname,'theme-source.json'),JSON.stringify({source:'front/app/assets/css/main.css',sha256:crypto.createHash('sha256').update(css).digest('hex'),selectors},null,2)+'\n');
console.log('Theme copied: canonical :root + editorial + dark; theme-tokens.css');
