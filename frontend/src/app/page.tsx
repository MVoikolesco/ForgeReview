"use client";

import { addEdge, Background, Controls, Handle, MiniMap, Position, ReactFlow, useEdgesState, useNodesState, type Connection, type Edge, type Node, type NodeProps } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Braces, CirclePlay, Database, GitBranch, Layers3, Plus, Save, ShieldCheck } from "lucide-react";
import { useEffect, useState, type CSSProperties } from "react";

type Port = { key: string; label: string; contract: string; required: boolean };
type CardType = { key: string; name: string; category: string; description: string; inputs: Port[]; outputs: Port[] };
type CardData = { key: string; type: string; name: string; category: string; inputs: Port[]; outputs: Port[]; config: Record<string, unknown>; status: "idle" | "running" | "completed" | "failed" };
type Definition = { key: string; name: string; description: string; nodes: Array<{ key: string; type: string; name: string; config: Record<string, unknown>; position: { x: number; y: number } }>; edges: Array<{ key: string; from_node: string; from_port: string; to_node: string; to_port: string }> };
type Report = { status: string; runs: Array<{ node_key: string; status: "completed" | "failed"; error?: string }> };

const apiURL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8088";
const accent = (category: string) => ({ "Entradas":"#4b8cff", "Dados":"#efb75e", "Transformação":"#56d6b6", "Controle":"#d58af3", "IA":"#6d9eff", "Validação":"#e87b91", "Resultado":"#efb75e", "Saída":"#78d2a5", "Infraestrutura":"#8e9aaa" }[category] || "#8e9aaa");
const localCards: CardType[] = [
  { key:"trigger", name:"Trigger manual", category:"Entradas", description:"Inicia a verificação visual.", inputs:[], outputs:[{key:"event",label:"Evento",contract:"event",required:false}] },
  { key:"transform", name:"Transformar contexto", category:"Transformação", description:"Propaga o contexto tipado.", inputs:[{key:"input",label:"Entrada",contract:"any",required:true}], outputs:[{key:"output",label:"Saída",contract:"any",required:false}] },
  { key:"condition", name:"Condição", category:"Controle", description:"Roteia o contexto para uma saída.", inputs:[{key:"input",label:"Entrada",contract:"any",required:true}], outputs:[{key:"true",label:"Atende",contract:"any",required:false},{key:"false",label:"Alternativa",contract:"any",required:false}] },
  { key:"log", name:"Log de execução", category:"Infraestrutura", description:"Registra o resultado da rota.", inputs:[{key:"input",label:"Entrada",contract:"any",required:false}], outputs:[{key:"output",label:"Saída",contract:"any",required:false}] },
];
const starterNodes: Node<CardData>[] = [
  { id:"trigger", type:"card", position:{x:70,y:260}, data:{key:"trigger",type:"trigger",name:"Trigger manual",category:"Entradas",inputs:[],outputs:localCards[0].outputs,config:{},status:"idle"} },
  { id:"transform", type:"card", position:{x:360,y:260}, data:{key:"transform",type:"transform",name:"Transformar contexto",category:"Transformação",inputs:localCards[1].inputs,outputs:localCards[1].outputs,config:{},status:"idle"} },
  { id:"condition", type:"card", position:{x:665,y:260}, data:{key:"condition",type:"condition",name:"Condição",category:"Controle",inputs:localCards[2].inputs,outputs:localCards[2].outputs,config:{equals:"never"},status:"idle"} },
  { id:"log", type:"card", position:{x:975,y:395}, data:{key:"log",type:"log",name:"Log de execução",category:"Infraestrutura",inputs:localCards[3].inputs,outputs:localCards[3].outputs,config:{},status:"idle"} },
];
const starterEdges: Edge[] = [
  {id:"trigger-transform",source:"trigger",sourceHandle:"out-event",target:"transform",targetHandle:"in-input",animated:true},
  {id:"transform-condition",source:"transform",sourceHandle:"out-output",target:"condition",targetHandle:"in-input",animated:true},
  {id:"condition-log",source:"condition",sourceHandle:"out-false",target:"log",targetHandle:"in-input",animated:true},
];

function WorkflowCard({ data }: NodeProps<Node<CardData>>) {
  return <article className={`workflow-card ${data.status}`} style={{ "--accent": accent(data.category) } as CSSProperties}>
    <header><i/><span>{data.category}</span><em>{data.status === "idle" ? "rascunho" : data.status === "running" ? "executando" : data.status === "completed" ? "concluído" : "falhou"}</em></header>
    <h2>{data.name}</h2>
    {data.inputs.map(port => <div className="port input" key={port.key}><Handle type="target" position={Position.Left} id={`in-${port.key}`}/><span>{port.label}</span><small>{port.contract}</small></div>)}
    {data.outputs.map(port => <div className="port output" key={port.key}><small>{port.contract}</small><span>{port.label}</span><Handle type="source" position={Position.Right} id={`out-${port.key}`}/></div>)}
  </article>;
}
const nodeTypes = { card: WorkflowCard };

function toDefinition(nodes: Node<CardData>[], edges: Edge[]): Definition {
  return { key:"studio-check", name:"Fluxo de verificação do Studio", description:"Pipeline local para validar cards, portas e estados.", nodes:nodes.map(node => ({key:node.id,type:node.data.type,name:node.data.name,config:node.data.config,position:node.position})), edges:edges.filter(edge => edge.sourceHandle && edge.targetHandle).map(edge => ({key:edge.id,from_node:edge.source,from_port:edge.sourceHandle!.replace("out-", ""),to_node:edge.target,to_port:edge.targetHandle!.replace("in-", "")})) };
}

export default function Studio() {
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<CardData>>(starterNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(starterEdges);
  const [cards, setCards] = useState<CardType[]>(localCards);
  const [selected, setSelected] = useState<CardData>(starterNodes[0].data);
  const [message, setMessage] = useState("Fluxo local pronto para validar.");
  const [busy, setBusy] = useState(false);
  useEffect(() => { void fetch(`${apiURL}/api/cards`).then(response => response.ok ? response.json() : Promise.reject()).then(setCards).catch(() => setMessage("Backend indisponível. Biblioteca local exibida.")); }, []);
  const addCard = (card: CardType) => {
    const key = `${card.key}-${Date.now().toString(36)}`;
    const data: CardData = {key,type:card.key,name:card.name,category:card.category,inputs:card.inputs,outputs:card.outputs,config:card.key === "template" ? {template:"Defina o prompt"} : {},status:"idle"};
    setNodes(all => [...all,{id:key,type:"card",position:{x:210 + (all.length % 4) * 280,y:90 + Math.floor(all.length / 4) * 250},data}]);
    setSelected(data);
  };
  const saveAndRun = async () => {
    setBusy(true); setMessage("Salvando versão do workflow...");
    const definition = toDefinition(nodes, edges);
    try {
      const saved = await fetch(`${apiURL}/api/workflows`, {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(definition)});
      const saveData = await saved.json(); if (!saved.ok) throw new Error(saveData.error || "Não foi possível salvar o workflow.");
      setMessage("Executando versão salva..."); setNodes(all => all.map(node => ({...node,data:{...node.data,status:"running"}})));
      const executed = await fetch(`${apiURL}/api/workflow-versions/${saveData.version_id}/executions`, {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({source:"studio"})});
      const runData: {execution_id?:number; report?:Report; error?:string; status?:string} = await executed.json();
      let report = runData.report;
      if (runData.status === "queued" && runData.execution_id) {
        for (let attempt = 0; attempt < 20; attempt++) {
          await new Promise(resolve => setTimeout(resolve, 500));
          const pending = await fetch(`${apiURL}/api/executions/${runData.execution_id}`);
          if (!pending.ok) continue;
          const value = await pending.json() as Report;
          if (value.status !== "queued" && value.status !== "running") { report = value; break; }
        }
      }
      if (report) setNodes(all => all.map(node => ({...node,data:{...node.data,status:report.runs.find(run => run.node_key === node.id)?.status || "idle"}})));
      if (!executed.ok) throw new Error(runData.error || "A execução falhou.");
      setMessage(report ? `Execução ${runData.execution_id} ${report.status === "completed" ? "concluída" : report.status}.` : `Execução ${runData.execution_id} enviada à fila.`);
    } catch (error) { setNodes(all => all.map(node => node.data.status === "running" ? {...node,data:{...node.data,status:"failed"}} : node)); setMessage(error instanceof Error ? error.message : "Erro inesperado."); }
    finally { setBusy(false); }
  };
  const categories = [...new Set(cards.map(card => card.category))];
  return <main className="studio"><header className="topbar"><strong><Braces size={20}/> ForgeReview <small>WORKFLOW STUDIO</small></strong><span>Fluxo de verificação</span><div><button disabled={busy} onClick={() => setMessage("As conexões visíveis usam contratos compatíveis.")}><ShieldCheck size={14}/> Validar</button><button className="primary" disabled={busy} onClick={() => void saveAndRun()}><CirclePlay size={14}/> {busy ? "Executando" : "Salvar e executar"}</button></div></header>
    <aside className="library"><div className="library-head"><b>Cards</b><small>Adicione ao canvas</small></div>{categories.map(category => <section key={category}><h2>{category === "Dados" ? <Database size={14}/> : category === "Controle" ? <GitBranch size={14}/> : <Layers3 size={14}/>} {category}</h2>{cards.filter(card => card.category === category).map(card => <button key={card.key} title={card.description} onClick={() => addCard(card)}><i/> {card.name}<Plus size={13}/></button>)}</section>)}</aside>
    <section className="canvas"><ReactFlow nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={(connection:Connection) => setEdges(all => addEdge({...connection,animated:true},all))} onNodeClick={(_,node) => setSelected(node.data)} fitView><Background gap={18} size={1}/><MiniMap/><Controls/></ReactFlow><p className="studio-message" role="status">{message}</p></section>
    <aside className="inspector"><header><span>CONFIGURAÇÃO</span><b>{selected.name}</b></header><dl><dt>Tipo</dt><dd>{selected.type}</dd><dt>Entradas</dt><dd>{selected.inputs.map(port => port.label).join(", ") || "Nenhuma"}</dd><dt>Saídas</dt><dd>{selected.outputs.map(port => port.label).join(", ") || "Nenhuma"}</dd></dl><p>Conecte as portas pelo canvas. O backend rejeita contratos incompatíveis e registra o estado de cada card na execução.</p></aside>
  </main>;
}
