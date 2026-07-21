"use client";

import { Background, Controls, Handle, MiniMap, Position, ReactFlow, type Node, type NodeProps } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Bot, Braces, Database, GitBranch, GitPullRequest, Layers3, ShieldCheck } from "lucide-react";

type CardData = { name: string; category: string; inputs: string[]; outputs: string[]; accent: string };
const nodes: Node<CardData>[] = [
  { id:"trigger", type:"card", position:{x:40,y:230}, data:{name:"Webhook Gitea",category:"ENTRADA",inputs:[],outputs:["Evento"],accent:"#4b8cff"} },
  { id:"fetch", type:"card", position:{x:330,y:230}, data:{name:"Buscar dados do PR",category:"DADOS",inputs:["Evento"],outputs:["PR", "Arquivos"],accent:"#efb75e"} },
  { id:"filter", type:"card", position:{x:635,y:100}, data:{name:"Filtro de arquivos",category:"TRANSFORMAÇÃO",inputs:["Arquivos"],outputs:["TypeScript", "PHP"],accent:"#d58af3"} },
  { id:"template-ts", type:"card", position:{x:950,y:35}, data:{name:"Prompt frontend",category:"TEMPLATE",inputs:["TypeScript"],outputs:["Prompt"],accent:"#56d6b6"} },
  { id:"model-ts", type:"card", position:{x:1245,y:35}, data:{name:"Gemma reviewer",category:"MODELO IA",inputs:["Prompt"],outputs:["Resposta"],accent:"#4b8cff"} },
  { id:"template-php", type:"card", position:{x:950,y:380}, data:{name:"Prompt backend",category:"TEMPLATE",inputs:["PHP"],outputs:["Prompt"],accent:"#56d6b6"} },
  { id:"model-php", type:"card", position:{x:1245,y:380}, data:{name:"GPT reviewer",category:"MODELO IA",inputs:["Prompt"],outputs:["Resposta"],accent:"#4b8cff"} },
  { id:"merge", type:"card", position:{x:1570,y:210}, data:{name:"Consolidar resultados",category:"RESULTADO",inputs:["Respostas"],outputs:["Review"],accent:"#efb75e"} },
];
const edges = [ ["trigger","fetch"], ["fetch","filter"], ["filter","template-ts"], ["filter","template-php"], ["template-ts","model-ts"], ["template-php","model-php"], ["model-ts","merge"], ["model-php","merge"] ].map(([source,target]) => ({id:`${source}-${target}`,source,target,animated:true}));
function Card({ data }: NodeProps<Node<CardData>>) { return <article className="workflow-card" style={{"--accent":data.accent} as React.CSSProperties}><header><i/><span>{data.category}</span><button aria-label="Configurar card">...</button></header><h2>{data.name}</h2>{data.inputs.map((item,index)=><div className="port input" key={item}><Handle type="target" position={Position.Left} id={`in-${index}`}/><span>{item}</span></div>)}{data.outputs.map((item,index)=><div className="port output" key={item}><span>{item}</span><Handle type="source" position={Position.Right} id={`out-${index}`}/></div>)}</article> }
const nodeTypes = { card: Card };
const sections = [["Entradas",GitPullRequest,["Trigger"]],["Dados",Database,["Buscar dados"]],["Transformação",Layers3,["Filtro","Agrupar","Template"]],["Controle",GitBranch,["Condição","Loop","Merge"]],["IA",Bot,["Modelo IA"]],["Validação",ShieldCheck,["Validar","Filtrar resposta"]]] as const;
export default function Studio() { return <main className="studio"><header className="topbar"><strong><Braces size={20}/> ForgeReview <small>WORKFLOW STUDIO</small></strong><span>Review de Pull Request</span><div><button>Validar</button><button className="primary">Publicar v1</button></div></header><aside className="library"><div className="library-head"><b>Cards</b><small>Arraste para o canvas</small></div>{sections.map(([title,Icon,cards])=><section key={title}><h2><Icon size={14}/>{title}</h2>{cards.map(card=><button key={card}><i/> {card}</button>)}</section>)}</aside><section className="canvas"><ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} fitView><Background gap={18} size={1}/><MiniMap/><Controls/></ReactFlow></section><aside className="inspector"><header><span>CONFIGURAÇÃO</span><b>Filtro de arquivos</b></header><label>Condição de inclusão<select><option>Extensão é .ts, .tsx, .js</option></select></label><label>Saída quando atende<input value="TypeScript" readOnly/></label><label>Saída alternativa<input value="PHP" readOnly/></label><p>As portas coloridas só aceitam contratos compatíveis.</p></aside></main> }
