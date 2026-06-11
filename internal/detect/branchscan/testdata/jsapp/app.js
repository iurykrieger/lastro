// Express-ish handler used by the heuristic analyzer test.
function createOrder(req, res) {
  if (!req.body.sku) {
    return res.status(400).json({ error: "missing sku" });
  } else if (req.body.qty <= 0) {
    return res.status(400).json({ error: "bad qty" });
  } else {
    persist(req.body);
  }

  switch (req.query.mode) {
    case "fast":
      enqueue(req.body);
      break;
    default:
      process(req.body);
  }

  try {
    notify(req.body);
  } catch (err) {
    log(err);
  }

  const label = req.body.gift ? "gift" : "standard";
  return res.status(201).json({ label });
}
