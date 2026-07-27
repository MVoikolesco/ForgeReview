# Subpipelines embutidas

O card `subpipeline` é um container estrutural persistido no mesmo workflow.
Ele organiza cards filhos sem substituir os cards executáveis ou criar um
reviewer genérico.

## Modelo

- `parent_key` associa um card ao container;
- `position` de um filho é relativa ao canto superior esquerdo do container;
- `size` persiste largura e altura do container;
- `config.input_ports` declara entradas;
- `config.output_ports` declara saídas.
- `config.instances`, quando presente, parametriza várias execuções da mesma
  receita interna sem duplicar seus cards no canvas.

Cada entrada possui dois lados com o mesmo contrato:

```text
pipeline principal -> entry:porta -> card interno
```

Cada saída segue o caminho inverso:

```text
card interno -> exit:porta -> pipeline principal
```

Uma conexão não pode atravessar diretamente a fronteira. Subpipelines também
não podem ser aninhadas nesta versão.

## Execução

O backend valida a estrutura persistida e, antes de executar, remove os
containers do grafo e combina os dois lados de cada fronteira. Assim:

- a subpipeline não produz uma execução própria;
- os logs e estados continuam pertencendo aos cards internos;
- contratos e políticas de cada card permanecem independentes;
- o runtime recebe um DAG comum, sem ciclos artificiais de entrada/saída.

## Receita parametrizada de revisão

A pipeline oficial persiste uma única subpipeline com cinco cards:

```text
Prompt + contrato → Modelo → Validar contrato → Confirmar → Filtrar
```

Essa receita aparece uma única vez e usa uma disposição compacta não linear.
Na configuração da própria subpipeline ficam as instâncias `Segurança`,
`Corretude`, `Contratos`, `Performance`, `Arquitetura` e `Observabilidade`.
Cada instância define:

- se está habilitada;
- prompt base;
- chave e versão do contrato;
- perfil do modelo reviewer e do modelo de confirmação;
- severidade mínima do filtro.

Somente durante a execução o backend clona temporariamente os cinco passos para
cada instância habilitada. As chaves recebem o sufixo `::instancia`, os logs
permanecem rastreáveis e o Studio agrega o progresso de volta nos cinco cards e
no container visíveis. Nenhum clone é salvo como card ou versão de workflow.

## Studio

O Studio permite:

- criar quantas subpipelines forem necessárias pelo catálogo;
- mover o container pelo cabeçalho;
- redimensioná-lo pelos controles de borda;
- arrastar cards sem conexões externas para dentro;
- remover cards pelo menu de contexto;
- editar nomes, chaves e contratos das portas;
- focar o container para trabalhar com os cards em tamanho normal.
- configurar, habilitar, adicionar e remover as execuções de uma receita de
  revisão no inspector do container.
